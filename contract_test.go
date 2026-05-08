package audd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixturesDir locates the audd-openapi fixtures directory via the
// AUDD_OPENAPI_FIXTURES env var. The contract suite is opt-in: when the env
// var is unset (the default for `go test ./...`), every contract test calls
// t.Skip(). The dedicated contract.yml CI job sets the env var to
// $GITHUB_WORKSPACE/audd-openapi/fixtures.
func fixturesDir(t *testing.T) string {
	t.Helper()
	v := os.Getenv("AUDD_OPENAPI_FIXTURES")
	if v == "" {
		t.Skip("AUDD_OPENAPI_FIXTURES env var not set; skipping contract tests. " +
			"Point it at the audd-openapi fixtures directory to run them.")
	}
	return v
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(fixturesDir(t), name)
	data, err := os.ReadFile(path) // nolint:gosec // test-only path
	require.NoErrorf(t, err, "loading fixture %s", path)
	return data
}

// loadResultRecognition pulls body.result and unmarshals into a Recognition.
func loadResultRecognition(t *testing.T, name string) *Recognition {
	t.Helper()
	body := loadFixture(t, name)
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &top))
	require.Contains(t, top, "result", "%s missing result", name)
	var rec Recognition
	require.NoError(t, json.Unmarshal(top["result"], &rec))
	return &rec
}

func TestContract_RecognizeBasic(t *testing.T) {
	r := loadResultRecognition(t, "recognize_basic.json")
	assert.Equal(t, "Tears For Fears", r.Artist)
	assert.Equal(t, "Everybody Wants To Rule The World", r.Title)
	assert.Equal(t, "00:56", r.Timecode)
	assert.Equal(t, "https://lis.tn/NbkVb", r.SongLink)
	assert.True(t, r.IsPublicMatch())
	assert.False(t, r.IsCustomMatch())
	assert.Equal(t, "https://lis.tn/NbkVb?thumb", r.ThumbnailURL())
}

func TestContract_RecognizeCustomMatch(t *testing.T) {
	r := loadResultRecognition(t, "recognize_custom_match.json")
	assert.Equal(t, "01:45", r.Timecode)
	require.NotNil(t, r.AudioID)
	assert.Equal(t, 146, *r.AudioID)
	assert.True(t, r.IsCustomMatch())
}

func TestContract_RecognizeWithMetadata_HasAppleAndSpotifyAndMusicBrainz(t *testing.T) {
	r := loadResultRecognition(t, "recognize_with_metadata.json")
	require.NotNil(t, r.AppleMusic)
	assert.Equal(t, "GBUM71403885", r.AppleMusic.ISRC)
	require.NotNil(t, r.Spotify)
	assert.NotEmpty(t, r.Spotify.URI)
	require.NotEmpty(t, r.MusicBrainz)
	assert.NotEmpty(t, r.MusicBrainz[0].ID)

	// Apple-Music payload has many extra fields (previews, artwork, etc.)
	// — they must all round-trip via Extras.
	assert.NotEmpty(t, r.AppleMusic.Extras, "Apple Music extras must round-trip")
}

func TestContract_EnterpriseWithIsrcUpc(t *testing.T) {
	body := loadFixture(t, "enterprise_with_isrc_upc.json")
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &top))
	var chunks []EnterpriseChunkResult
	require.NoError(t, json.Unmarshal(top["result"], &chunks))
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Songs, 1)
	m := chunks[0].Songs[0]
	assert.Equal(t, "Tears For Fears", m.Artist)
	assert.Equal(t, "GBUM71403885", m.ISRC)
	assert.Equal(t, "00602547037169", m.UPC)
}

// errFromFixtureMatchesSentinel re-runs the fixture body through
// raiseFromErrorResponse to verify code → sentinel mapping.
func errFromFixture(t *testing.T, name string) error {
	t.Helper()
	body := loadFixture(t, name)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Equal(t, "error", raw["status"])
	return raiseFromErrorResponse(raw, 200, "", false)
}

func TestContract_Error900_Authentication(t *testing.T) {
	err := errFromFixture(t, "error_900_invalid_token.json")
	assert.ErrorIs(t, err, ErrAuthentication)
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 900, apiErr.ErrorCode)
	assert.Contains(t, apiErr.RequestedParams, "api_token")
}

func TestContract_Error902_QuotaOnAddStream(t *testing.T) {
	err := errFromFixture(t, "error_902_stream_limit.json")
	assert.ErrorIs(t, err, ErrQuota)
}

func TestContract_Error904_EnterpriseUnauthorized(t *testing.T) {
	err := errFromFixture(t, "error_904_enterprise_unauthorized.json")
	// Without custom-catalog context, code 904 maps to Subscription.
	assert.ErrorIs(t, err, ErrSubscription)
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	// "requested_params" alt-spelling is normalized.
	assert.Contains(t, apiErr.RequestedParams, "url")
}

func TestContract_Error700_NoFile(t *testing.T) {
	err := errFromFixture(t, "error_700_no_file.json")
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestContract_Error19_NoCallbackURL(t *testing.T) {
	err := errFromFixture(t, "error_19_no_callback_url.json")
	// Code 19 maps to ErrBlocked in our default mapping; the longpoll
	// preflight separately surfaces a clearer hint at the call-site.
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 19, apiErr.ErrorCode)
}

func TestContract_StreamCallback_Result(t *testing.T) {
	body := loadFixture(t, "streams_callback_with_result.json")
	match, notif, err := ParseCallback(body)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.Nil(t, notif)
	assert.Equal(t, int64(7), match.RadioID)
	assert.Equal(t, "Alan Walker, A$AP Rocky", match.Song.Artist)
}

func TestContract_StreamCallback_Notification(t *testing.T) {
	body := loadFixture(t, "streams_callback_with_notification.json")
	match, notif, err := ParseCallback(body)
	require.NoError(t, err)
	require.Nil(t, match)
	require.NotNil(t, notif)
	assert.Equal(t, 3, notif.RadioID)
	assert.Equal(t, 650, notif.NotificationCode)
}

func TestContract_LongpollNoEvents(t *testing.T) {
	body := loadFixture(t, "longpoll_no_events.json")
	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	assert.Equal(t, "no events before timeout", raw["timeout"])
}

func TestContract_GetStreamsEmpty(t *testing.T) {
	body := loadFixture(t, "getStreams_empty.json")
	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	assert.Equal(t, "success", raw["status"])
	result, _ := raw["result"].([]any)
	assert.Empty(t, result)
}

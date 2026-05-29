package audd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecognition_PublicMatch(t *testing.T) {
	data := []byte(`{
		"timecode": "00:56", "artist": "Tears For Fears",
		"title": "Everybody Wants To Rule The World",
		"song_link": "https://lis.tn/NbkVb"
	}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Equal(t, "Tears For Fears", r.Artist)
	assert.True(t, r.IsPublicMatch())
	assert.False(t, r.IsCustomMatch())
}

func TestRecognition_CustomMatch(t *testing.T) {
	data := []byte(`{"timecode": "01:45", "audio_id": 146}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.True(t, r.IsCustomMatch())
	assert.False(t, r.IsPublicMatch())
	require.NotNil(t, r.AudioID)
	assert.Equal(t, 146, *r.AudioID)
}

func TestRecognition_ThumbnailURL(t *testing.T) {
	cases := []struct {
		songLink string
		want     string
	}{
		{"https://lis.tn/NbkVb", "https://lis.tn/NbkVb?thumb"},
		{"https://lis.tn/x?foo=1", "https://lis.tn/x?foo=1&thumb"},
		{"https://www.youtube.com/watch?v=abc", ""},
		{"", ""},
	}
	for _, c := range cases {
		r := Recognition{SongLink: c.songLink}
		assert.Equal(t, c.want, r.ThumbnailURL(), "song_link=%q", c.songLink)
	}
}

func TestExtras_CapturesUnknownKeys(t *testing.T) {
	data := []byte(`{
		"timecode": "00:56", "artist": "X", "title": "Y",
		"new_field_2027": "future-proof", "extra_block": {"a": 1}
	}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Contains(t, r.Extras, "new_field_2027")
	assert.Contains(t, r.Extras, "extra_block")
	assert.NotContains(t, r.Extras, "timecode", "known keys must not be in extras")
}

func TestRecognition_RawResponse_RoundTrips(t *testing.T) {
	data := []byte(`{"timecode":"00:56","artist":"X"}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.JSONEq(t, string(data), string(r.RawResponse))
}

func TestEnterpriseMatch_ParsesISRCAndUPC(t *testing.T) {
	data := []byte(`{
		"score": 100, "timecode": "00:00",
		"artist": "X", "title": "Y", "isrc": "US0001", "upc": "00000000"
	}`)
	var m EnterpriseMatch
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "US0001", m.ISRC)
	assert.Equal(t, "00000000", m.UPC)
}

// TestEnterpriseMatch_OmittedScoreISRCUPC verifies the CEO rule: a successful
// enterprise response whose song omits score/isrc/upc/label decodes without
// error, leaving those fields at their zero values while the present fields
// populate normally. The enterprise endpoint legitimately returns matches
// with no score and no isrc/upc/label.
func TestEnterpriseMatch_OmittedScoreISRCUPC(t *testing.T) {
	// Full enterprise-shaped result: array of chunks, each with a songs array.
	// The song deliberately omits score, isrc, upc, and label.
	body := []byte(`[{"offset":"0","songs":[{"timecode":"00:00","artist":"X","title":"Y","song_link":"https://lis.tn/abc"}]}]`)

	var chunks []EnterpriseChunkResult
	require.NoError(t, json.Unmarshal(body, &chunks))
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Songs, 1)

	m := chunks[0].Songs[0]
	assert.Equal(t, "X", m.Artist)
	assert.Equal(t, "Y", m.Title)
	assert.Equal(t, "https://lis.tn/abc", m.SongLink)
	// Absent fields are zero values, not errors.
	assert.Equal(t, 0, m.Score)
	assert.Equal(t, "", m.ISRC)
	assert.Equal(t, "", m.UPC)
	assert.Equal(t, "", m.Label)
}

// TestParseCallback_EmptyResults verifies a recognition callback whose
// `results` array is absent or empty parses without error: Song stays its
// zero value and Alternatives is empty.
func TestParseCallback_EmptyResults(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"status":"success","result":{"radio_id":7,"timestamp":"x","results":[]}}`),
		[]byte(`{"status":"success","result":{"radio_id":7,"timestamp":"x"}}`),
	}
	for _, body := range cases {
		match, notif, err := ParseCallback(body)
		require.NoError(t, err)
		require.Nil(t, notif)
		require.NotNil(t, match)
		assert.Equal(t, int64(7), match.RadioID)
		assert.Equal(t, StreamCallbackSong{}, match.Song)
		assert.Empty(t, match.Alternatives)
	}
}

func TestStream_ParsesAndCapturesExtras(t *testing.T) {
	data := []byte(`{
		"radio_id": 7, "url": "twitch:foo", "stream_running": true,
		"longpoll_category": "abc",
		"undocumented_flag": true
	}`)
	var s Stream
	require.NoError(t, json.Unmarshal(data, &s))
	assert.Equal(t, 7, s.RadioID)
	assert.True(t, s.StreamRunning)
	assert.Contains(t, s.Extras, "undocumented_flag")
}

func TestLyricsResult_Parses(t *testing.T) {
	data := []byte(`{
		"artist": "A", "title": "T", "lyrics": "la la la",
		"song_id": 1, "song_link": "https://x"
	}`)
	var l LyricsResult
	require.NoError(t, json.Unmarshal(data, &l))
	assert.Equal(t, "A", l.Artist)
	assert.Equal(t, "la la la", l.Lyrics)
}

func TestEnterpriseMatch_ThumbnailURL(t *testing.T) {
	m := EnterpriseMatch{SongLink: "https://lis.tn/abc"}
	assert.Equal(t, "https://lis.tn/abc?thumb", m.ThumbnailURL())
	m2 := EnterpriseMatch{SongLink: "https://www.youtube.com/x"}
	assert.Equal(t, "", m2.ThumbnailURL())
}

func TestAppleMusicMetadata_Extras(t *testing.T) {
	data := []byte(`{
		"artistName": "X", "name": "T", "previews": [{"url": "https://x"}],
		"artwork": {"url": "https://y"}
	}`)
	var a AppleMusicMetadata
	require.NoError(t, json.Unmarshal(data, &a))
	assert.Equal(t, "X", a.ArtistName)
	assert.Contains(t, a.Extras, "previews")
	assert.Contains(t, a.Extras, "artwork")
}

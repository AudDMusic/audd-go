package audd

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveLongpollCategory_StableAndCorrectLength(t *testing.T) {
	cat := DeriveLongpollCategory("test", 7)
	assert.Len(t, cat, 9)
	// Stable: same inputs → same output.
	assert.Equal(t, cat, DeriveLongpollCategory("test", 7))
	// Different inputs → different output.
	assert.NotEqual(t, cat, DeriveLongpollCategory("test", 8))
	assert.NotEqual(t, cat, DeriveLongpollCategory("other", 7))
}

func TestDeriveLongpollCategory_OnlyHexChars(t *testing.T) {
	cat := DeriveLongpollCategory("test", 7)
	for _, r := range cat {
		assert.True(t,
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'),
			"unexpected non-hex char %q in %q", r, cat,
		)
	}
}

func TestParseCallback_Result(t *testing.T) {
	body := []byte(`{"status":"success","result":{"radio_id":7,"timestamp":"2020-04-13 10:31:43","play_length":111,"results":[{"artist":"A","title":"T","score":100}]}}`)
	p, err := ParseCallback(body)
	require.NoError(t, err)
	require.True(t, p.IsResult())
	assert.Equal(t, 7, p.Result.RadioID)
	assert.Equal(t, 111, p.Result.PlayLength)
	require.Len(t, p.Result.Results, 1)
	assert.Equal(t, "A", p.Result.Results[0].Artist)
}

func TestParseCallback_Notification(t *testing.T) {
	body := []byte(`{"status":"-","notification":{"radio_id":3,"stream_running":false,"notification_code":650,"notification_message":"can't connect"},"time":1587939136}`)
	p, err := ParseCallback(body)
	require.NoError(t, err)
	require.True(t, p.IsNotification())
	assert.Equal(t, 3, p.Notification.RadioID)
	require.NotNil(t, p.Notification.StreamRunning)
	assert.False(t, *p.Notification.StreamRunning)
	assert.Equal(t, 650, p.Notification.NotificationCode)
	assert.Equal(t, 1587939136, p.Time)
}

func TestParseCallback_BadJSON(t *testing.T) {
	_, err := ParseCallback([]byte(`not json`))
	require.Error(t, err)
}

func TestParseCallback_NeitherResultNorNotification(t *testing.T) {
	_, err := ParseCallback([]byte(`{"foo":"bar"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither")
}

func TestAddReturnToURL_Empty_NoChange(t *testing.T) {
	got, err := addReturnToURL("https://x", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://x", got)
}

func TestAddReturnToURL_AppendsCSV(t *testing.T) {
	got, err := addReturnToURL("https://x", []string{"apple_music", "spotify"})
	require.NoError(t, err)
	parsed, _ := url.Parse(got)
	assert.Equal(t, "apple_music,spotify", parsed.Query().Get("return"))
}

func TestAddReturnToURL_PreservesExistingQuery(t *testing.T) {
	got, err := addReturnToURL("https://x?foo=1", []string{"deezer"})
	require.NoError(t, err)
	parsed, _ := url.Parse(got)
	assert.Equal(t, "1", parsed.Query().Get("foo"))
	assert.Equal(t, "deezer", parsed.Query().Get("return"))
}

func TestAddReturnToURL_DuplicateReturn_ErrsOut(t *testing.T) {
	_, err := addReturnToURL("https://x?return=apple_music", []string{"spotify"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already contains")
}

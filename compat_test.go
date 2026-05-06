package audd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify the deprecated flat API still compiles AND routes to the
// new namespaced impl. Per project rule: keep these working through v2.0.0.

func TestCompat_RecognizeByUrl_RoutesToRecognize(t *testing.T) {
	var seenReturn string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenReturn = r.FormValue("return")
		_, _ = w.Write([]byte(`{"status":"success","result":{"timecode":"00:01","artist":"X","title":"Y"}}`))
	})
	defer func() { _ = c.Close() }()

	res, err := c.RecognizeByUrl("https://example.com/song.mp3", "apple_music,spotify", nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "X", res.Artist)
	assert.Equal(t, "apple_music,spotify", seenReturn)
}

func TestCompat_RecognizeByFile_RoutesToRecognize(t *testing.T) {
	var sawFile bool
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		_, _, err := r.FormFile("file")
		sawFile = err == nil
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeByFile(strings.NewReader("audio-bytes"), "", nil)
	require.NoError(t, err)
	assert.True(t, sawFile)
}

func TestCompat_FindLyrics_RoutesToAdvanced(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/findLyrics/", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"success","result":[{"artist":"A","title":"T"}]}`))
	})
	defer func() { _ = c.Close() }()

	out, err := c.FindLyrics("hello", nil)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "A", out[0].Artist)
}

func TestCompat_AddStream_RoutesToStreams(t *testing.T) {
	var seenURL, seenRadio string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/addStream/", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenURL = r.FormValue("url")
		seenRadio = r.FormValue("radio_id")
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.AddStream("twitch:foo", 7, "before", nil))
	assert.Equal(t, "twitch:foo", seenURL)
	assert.Equal(t, "7", seenRadio)
}

func TestCompat_SetCallbackUrl_GetCallbackUrl(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":true}`))
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://x"}`))
		}
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.SetCallbackUrl("https://x"))
	got, err := c.GetCallbackUrl()
	require.NoError(t, err)
	assert.Equal(t, "https://x", got)
}

func TestCompat_SetStreamUrl_DeleteStream_GetStreams(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setStreamUrl/", "/deleteStream/":
			_, _ = w.Write([]byte(`{"status":"success","result":true}`))
		case "/getStreams/":
			_, _ = w.Write([]byte(`{"status":"success","result":[{"radio_id":1,"url":"twitch:a","stream_running":true}]}`))
		}
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.SetStreamUrl(1, "https://x"))
	require.NoError(t, c.DeleteStream(1))
	streams, err := c.GetStreams()
	require.NoError(t, err)
	require.Len(t, streams, 1)
}

func TestCompat_AddSongToCustomDB_RoutesToCustomCatalog(t *testing.T) {
	var seenAudioID string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/upload/", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenAudioID = r.FormValue("audio_id")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})
	defer func() { _ = c.Close() }()
	require.NoError(t, c.AddSongToCustomDB(99, []byte("x")))
	assert.Equal(t, "99", seenAudioID)
}

// SongIsRecognitionAlias verifies the legacy Song type alias works.
func TestCompat_SongIsRecognitionAlias(t *testing.T) {
	var s *Song
	var r *Recognition = s // compiles only if Song = Recognition
	_ = r
}

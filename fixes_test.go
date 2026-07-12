package audd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Lenient parsing: wrong-typed response fields degrade to zero values ---

func TestLenient_Recognition_CoercesConvertibleFields(t *testing.T) {
	// Numeric timecode, string audio_id, numeric artist: none may fail the
	// decode; convertible values coerce to the field's type.
	data := []byte(`{"timecode": 56, "audio_id": "146", "artist": 42, "title": "Y", "album": "Z"}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Equal(t, "56", r.Timecode, "numeric timecode renders to string")
	require.NotNil(t, r.AudioID, "numeric-string audio_id parses")
	assert.Equal(t, 146, *r.AudioID)
	assert.Equal(t, "42", r.Artist, "numeric artist renders to string")
	assert.Equal(t, "Y", r.Title)
	assert.Equal(t, "Z", r.Album)
}

func TestLenient_Recognition_UnconvertibleFieldsDegrade(t *testing.T) {
	// Garbage that can't be coerced degrades to the zero value — never an
	// error, never a misleading zero from a partial parse.
	data := []byte(`{"audio_id": "abc", "timecode": {"x": 1}, "artist": ["A"], "title": "Y"}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Nil(t, r.AudioID, "non-numeric string does not coerce to int")
	assert.Equal(t, "", r.Timecode, "object does not coerce to string")
	assert.Equal(t, "", r.Artist, "array does not coerce to string")
	assert.Equal(t, "Y", r.Title)
}

func TestLenient_Recognition_WrongTypedMetadataBlock(t *testing.T) {
	// apple_music as a string (not an object) must not fail the whole result.
	data := []byte(`{"artist": "X", "title": "Y", "apple_music": "oops"}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Nil(t, r.AppleMusic)
	assert.Equal(t, "X", r.Artist)
}

func TestLenient_Recognition_WrongTypedNestedMetadataField(t *testing.T) {
	data := []byte(`{"artist": "X", "apple_music": {"artistName": "X", "durationInMillis": "180000"}}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	require.NotNil(t, r.AppleMusic)
	assert.Equal(t, "X", r.AppleMusic.ArtistName)
	assert.Equal(t, 180000, r.AppleMusic.DurationInMillis, "numeric-string nested field coerces")
}

func TestLenient_RecognizeCall_WrongTypedFieldDoesNotFail(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":{"timecode":56,"artist":"X","title":"Y"}}`))
	})
	defer func() { _ = c.Close() }()

	res, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.NoError(t, err, "wrong-typed timecode must not fail the call")
	require.NotNil(t, res)
	assert.Equal(t, "X", res.Artist)
	assert.Equal(t, "56", res.Timecode)
}

func TestLenient_EnterpriseMatch_StringScore(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[
			{"offset":"0","songs":[{"score":"85","timecode":7,"artist":"A","title":"T1"}]}
		]}`))
	})
	defer func() { _ = c.Close() }()

	matches, err := c.RecognizeEnterpriseContext(context.Background(), "https://example.com/big.mp3", nil)
	require.NoError(t, err, "string score / numeric timecode must not fail the call")
	require.Len(t, matches, 1)
	assert.Equal(t, 85, matches[0].Score, `{"score":"85"} coerces to 85`)
	assert.Equal(t, "7", matches[0].Timecode)
	assert.Equal(t, "A", matches[0].Artist)
}

func TestLenient_ScalarCoercionMatrix(t *testing.T) {
	cases := []struct {
		name string
		body string
		want EnterpriseMatch
	}{
		{"float score truncates", `{"score": 85.9}`, EnterpriseMatch{Score: 85}},
		{"float-string score truncates", `{"score": "8.5"}`, EnterpriseMatch{Score: 8}},
		{"bool score maps to 1", `{"score": true}`, EnterpriseMatch{Score: 1}},
		{"whitespace numeric string parses", `{"score": " 85 "}`, EnterpriseMatch{Score: 85}},
		{"partial numeric string degrades", `{"score": "85abc"}`, EnterpriseMatch{Score: 0}},
		{"NaN string degrades", `{"score": "NaN"}`, EnterpriseMatch{Score: 0}},
		{"Infinity string degrades", `{"score": "Infinity"}`, EnterpriseMatch{Score: 0}},
		{"bool artist renders", `{"artist": true}`, EnterpriseMatch{Artist: "true"}},
	}
	for _, tc := range cases {
		var m EnterpriseMatch
		require.NoError(t, json.Unmarshal([]byte(tc.body), &m), tc.name)
		assert.Equal(t, tc.want.Score, m.Score, tc.name)
		assert.Equal(t, tc.want.Artist, m.Artist, tc.name)
	}
}

func TestLenient_Stream_CoercesWrongTypedFields(t *testing.T) {
	data := []byte(`{"radio_id": "9", "url": "twitch:foo", "stream_running": "true"}`)
	var s Stream
	require.NoError(t, json.Unmarshal(data, &s))
	assert.Equal(t, 9, s.RadioID, "numeric-string radio_id parses")
	assert.True(t, s.StreamRunning, `"true" coerces to true`)
	assert.Equal(t, "twitch:foo", s.URL)
}

func TestLenient_BoolCoercionWhitelist(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`"true"`, true}, {`"1"`, true}, {`"YES"`, true}, {`"on"`, true},
		{`"false"`, false}, {`"0"`, false}, {`"No"`, false}, {`"off"`, false}, {`""`, false},
		{`1`, true}, {`0`, false},
	} {
		var s Stream
		require.NoError(t, json.Unmarshal([]byte(`{"stream_running": `+tc.raw+`}`), &s))
		assert.Equal(t, tc.want, s.StreamRunning, "stream_running=%s", tc.raw)
	}
	// Unrecognized strings degrade to the zero value, never guess true.
	var s Stream
	require.NoError(t, json.Unmarshal([]byte(`{"stream_running": "maybe"}`), &s))
	assert.False(t, s.StreamRunning)
}

func TestLenient_StreamsList_WrongTypedFieldDoesNotFail(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[{"radio_id":"1","url":"twitch:a","stream_running":true}]}`))
	})
	defer func() { _ = c.Close() }()

	streams, err := c.Streams().List()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	assert.Equal(t, 1, streams[0].RadioID, "numeric-string radio_id coerces")
	assert.True(t, streams[0].StreamRunning)
}

func TestLenient_Callback_CoercesWrongTypedFields(t *testing.T) {
	body := []byte(`{"status":"success","result":{
		"radio_id": "7", "timestamp": 12345, "play_length": "220",
		"results": [{"artist": "A", "title": "T", "score": "99"}]
	}}`)
	match, notif, err := ParseCallback(body)
	require.NoError(t, err, "wrong-typed callback fields must not fail parsing")
	require.Nil(t, notif)
	require.NotNil(t, match)
	assert.Equal(t, int64(7), match.RadioID)
	assert.Equal(t, "12345", match.Timestamp)
	assert.Equal(t, 220, match.PlayLength)
	assert.Equal(t, "A", match.Song.Artist)
	assert.Equal(t, 99, match.Song.Score)
}

func TestLenient_CallbackNotification_CoercesWrongTypedFields(t *testing.T) {
	body := []byte(`{"notification":{
		"radio_id": 3, "stream_running": "false",
		"notification_code": "650", "notification_message": "can't connect"
	},"time":1587939136}`)
	match, notif, err := ParseCallback(body)
	require.NoError(t, err)
	require.Nil(t, match)
	require.NotNil(t, notif)
	assert.Equal(t, 3, notif.RadioID)
	require.NotNil(t, notif.StreamRunning, `"false" coerces through the pointer`)
	assert.False(t, *notif.StreamRunning)
	assert.Equal(t, 650, notif.NotificationCode)
	assert.Equal(t, "can't connect", notif.NotificationMessage)
}

func TestLenient_LyricsResult_WrongTypedSongID(t *testing.T) {
	data := []byte(`{"artist": "A", "title": "T", "song_id": "abc"}`)
	var l LyricsResult
	require.NoError(t, json.Unmarshal(data, &l))
	assert.Equal(t, 0, l.SongID, "non-numeric string degrades")
	assert.Equal(t, "A", l.Artist)
}

// URL helpers must resolve from the typed metadata blocks a real payload
// decodes into (metadata keys are known keys and never land in Extras).
func TestStreamingURL_ResolvesFromDecodedMetadataBlocks(t *testing.T) {
	data := []byte(`{
		"artist": "X", "title": "Y", "song_link": "https://www.youtube.com/watch?v=x",
		"apple_music": {"url": "https://music.apple.com/us/album/x/123",
			"previews": [{"url": "https://itunes/preview.m4a"}]},
		"spotify": {"external_urls": {"spotify": "https://open.spotify.com/track/abc"}},
		"deezer": {"link": "https://www.deezer.com/track/123", "preview": "https://deezer/preview.mp3"}
	}`)
	var r Recognition
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Equal(t, "https://music.apple.com/us/album/x/123", r.StreamingURL(ProviderAppleMusic))
	assert.Equal(t, "https://open.spotify.com/track/abc", r.StreamingURL(ProviderSpotify))
	assert.Equal(t, "https://www.deezer.com/track/123", r.StreamingURL(ProviderDeezer))
	assert.Equal(t, "https://itunes/preview.m4a", r.PreviewURL())
}

// --- Data races ---

func TestRace_ConcurrentSubClientAccess(t *testing.T) {
	c := NewClient("test")
	defer func() { _ = c.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NotNil(t, c.Streams())
			assert.NotNil(t, c.CustomCatalog())
			assert.NotNil(t, c.Advanced())
		}()
	}
	wg.Wait()
	// Same instance across accesses.
	assert.Same(t, c.Streams(), c.Streams())
}

func TestRace_DeriveLongpollCategoryDuringTokenRotation(t *testing.T) {
	c := NewClient("first-token")
	defer func() { _ = c.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = c.Streams().DeriveLongpollCategory(7)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				require.NoError(t, c.SetAPIToken("second-token"))
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, DeriveLongpollCategory("second-token", 7), c.Streams().DeriveLongpollCategory(7))
}

// --- Per-call Timeout options ---

func TestRecognize_TimeoutOptionBoundsTheCall(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	start := time.Now()
	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3",
		&RecognizeOptions{Timeout: 50 * time.Millisecond})
	require.Error(t, err, "the per-call timeout must cancel the request")
	assert.Less(t, time.Since(start), 300*time.Millisecond)

	// Without the option the same call succeeds.
	_, err = c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.NoError(t, err)
}

func TestRecognizeEnterprise_TimeoutOptionBoundsTheCall(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	limit := 1
	_, err := c.RecognizeEnterpriseContext(context.Background(), "https://example.com/big.mp3",
		&EnterpriseOptions{Limit: &limit, Timeout: 50 * time.Millisecond})
	require.Error(t, err, "the per-call timeout must cancel the request")
}

// --- File handles: SDK-opened files are closed; caller readers are not ---

type countingCloser struct {
	io.Reader
	closes int
}

func (c *countingCloser) Close() error {
	c.closes++
	return nil
}

func TestPostForm_ClosesOwnedReader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)

	h := newHTTPClient("test", 0, srv.Client())
	cc := &countingCloser{Reader: strings.NewReader("audio-bytes")}
	_, err := h.postForm(context.Background(), srv.URL, formFields{
		File: &fileField{Name: "upload.bin", Reader: cc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, cc.closes, "postForm must close a closable file reader after the attempt")
}

func TestRecognize_FilePathSource_NoFDLeakAcrossRetries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp3")
	require.NoError(t, os.WriteFile(path, []byte("audio-bytes"), 0o600))

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("test", WithMaxAttempts(3), WithBackoffFactor(time.Millisecond))
	c.standardHTTP = newHTTPClient("test", 0, &http.Client{Transport: rewriteTransport{base: srv.URL}})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), path, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.Zero(t, openFDsForPath(t, path), "every per-attempt file open must be matched by a close")
}

// openFDsForPath counts this process's file descriptors that point at path.
func openFDsForPath(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("no /proc/self/fd on this platform")
	}
	n := 0
	for _, e := range entries {
		if target, err := os.Readlink(filepath.Join("/proc/self/fd", e.Name())); err == nil && target == path {
			n++
		}
	}
	return n
}

func TestRecognize_CallerReaderIsNotClosed(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	cc := &countingCloser{Reader: strings.NewReader("audio-bytes")}
	_, err := c.RecognizeContext(context.Background(), io.Reader(cc), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, cc.closes, "the SDK must not close caller-supplied readers")
}

// --- v0 compat shims forward their options ---

func TestCompat_RecognizeByUrl_ForwardsLegacyOptions(t *testing.T) {
	var seenMarket, seenSkip, seenReturn string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenMarket = r.FormValue("market")
		seenSkip = r.FormValue("skip")
		seenReturn = r.FormValue("return")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeByUrl("https://example.com/song.mp3", "apple_music",
		map[string]string{"market": "de", "skip": "3"})
	require.NoError(t, err)
	assert.Equal(t, "de", seenMarket)
	assert.Equal(t, "3", seenSkip, "non-market legacy options must be forwarded")
	assert.Equal(t, "apple_music", seenReturn)
}

func TestCompat_FindLyrics_ForwardsOptions(t *testing.T) {
	var seenQ, seenExtra string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenQ = r.FormValue("q")
		seenExtra = r.FormValue("some_param")
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.FindLyrics("hello", map[string]string{"some_param": "v"})
	require.NoError(t, err)
	assert.Equal(t, "hello", seenQ)
	assert.Equal(t, "v", seenExtra, "legacy findLyrics options must be forwarded")
}

func TestCompat_AddStream_ForwardsOptions(t *testing.T) {
	var seenURL, seenExtra string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenURL = r.FormValue("url")
		seenExtra = r.FormValue("some_param")
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.AddStream("twitch:foo", 7, "", map[string]string{"some_param": "v"}))
	assert.Equal(t, "twitch:foo", seenURL)
	assert.Equal(t, "v", seenExtra, "legacy addStream options must be forwarded")
}

// --- OnEvent fires for non-recognize calls too ---

func TestOnEvent_FiresForStreamsAndAdvancedCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	}))
	t.Cleanup(srv.Close)

	var mu sync.Mutex
	var methods []string
	c := mkPolishClientWithEvent("t", srv.URL, func(e AudDEvent) {
		if e.Kind == "response" {
			mu.Lock()
			methods = append(methods, e.Method)
			mu.Unlock()
		}
	})
	defer func() { _ = c.Close() }()

	_, err := c.Streams().List()
	require.NoError(t, err)
	_, err = c.Advanced().RawRequest("someMethod", nil)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, methods, "getStreams", "streams calls must emit events")
	assert.Contains(t, methods, "someMethod", "advanced raw calls must emit events")
}

// --- Longpoll preflight: only the no-callback-URL #19 is rewritten ---

func TestStreams_Longpoll_PreflightPassesThroughOtherCode19(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getCallbackUrl/", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":19,"error_message":"Scheduled maintenance, try again later"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Streams().LongpollContext(context.Background(), "cat", nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no callback URL", "a real #19 must pass through, not be rewritten")
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 19, apiErr.ErrorCode)
	assert.Contains(t, apiErr.Message, "maintenance")
}

func TestStreams_Longpoll_PreflightRewritesInternalErrorSignal(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":19,"error_message":"Internal error"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Streams().LongpollContext(context.Background(), "cat", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no callback URL is configured")
}

// --- Longpoll requests are sized to the poll timeout + margin ---

func TestLongpollRequestTimeout_SizesAbovePollTimeout(t *testing.T) {
	assert.Equal(t, 50*time.Second+longpollTimeoutMargin, longpollRequestTimeout(50))
	assert.Equal(t, 300*time.Second+longpollTimeoutMargin, longpollRequestTimeout(300))
}

func TestLongpollHTTPClient_HasNoClientLevelTimeout(t *testing.T) {
	c := NewClient("test")
	defer func() { _ = c.Close() }()
	require.True(t, c.longpollHTTP.owned)
	assert.Equal(t, time.Duration(0), c.longpollHTTP.hc.Timeout,
		"longpoll transport must not cap requests at the standard timeout; the per-request deadline bounds each poll")

	lc := NewLongpollConsumer("cat")
	defer func() { _ = lc.Close() }()
	require.True(t, lc.httpc.owned)
	assert.Equal(t, time.Duration(0), lc.httpc.hc.Timeout)
}

func TestStreams_Longpoll_TimeoutAbove60sStillPolls(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			assert.Equal(t, "90", r.URL.Query().Get("timeout"))
			_, _ = w.Write([]byte(`{"status":"success","result":{"radio_id":7,"timestamp":"x","results":[{"artist":"A","title":"T","score":99}]}}`))
		}
	})
	defer func() { _ = c.Close() }()

	p, err := c.Streams().LongpollContext(context.Background(), "cat", &LongpollOptions{Timeout: 90})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()
	select {
	case m := <-p.Matches:
		assert.Equal(t, "A", m.Song.Artist)
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a match")
	}
}

// --- FindLyrics decode path: code-51 pass-through + request metadata ---

func TestAdvanced_FindLyrics_Code51PassThrough(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":51,"error_message":"deprecated param"},"result":[{"artist":"A","title":"T"}]}`))
	})
	defer func() { _ = c.Close() }()
	var deprecMsg string
	c.onDeprecation = func(msg string) { deprecMsg = msg }

	out, err := c.Advanced().FindLyrics("q")
	require.NoError(t, err, "code 51 + result must pass through for findLyrics too")
	require.Len(t, out, 1)
	assert.Equal(t, "A", out[0].Artist)
	assert.Contains(t, deprecMsg, "deprecated")
}

func TestAdvanced_FindLyrics_ErrorCarriesHTTPStatusAndRequestID(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "rid-7")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":300,"error_message":"no q"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Advanced().FindLyrics("")
	require.Error(t, err)
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.HTTPStatus, "findLyrics errors must carry the real HTTP status")
	assert.Equal(t, "rid-7", apiErr.RequestID, "findLyrics errors must carry the request ID")
}

// --- Offsets above one hour ---

func TestParseOffsetToSeconds_AboveOneHour(t *testing.T) {
	v, ok := parseOffsetToSeconds("01:02:03")
	require.True(t, ok)
	assert.InDelta(t, 3723, v, 1e-9)
}

package audd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreams_SetCallbackUrl_Posts(t *testing.T) {
	var seenPath, seenURL string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenURL = r.FormValue("url")
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.Streams().SetCallbackUrl("https://example.com/cb", nil))
	assert.Equal(t, "/setCallbackUrl/", seenPath)
	assert.Equal(t, "https://example.com/cb", seenURL)
}

func TestStreams_SetCallbackUrl_AppendsReturnMetadata(t *testing.T) {
	var seenURL string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenURL = r.FormValue("url")
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.Streams().SetCallbackUrl("https://example.com/cb",
		&SetCallbackUrlOptions{ReturnMetadata: []string{"apple_music", "spotify"}}))
	parsed, _ := url.Parse(seenURL)
	assert.Equal(t, "apple_music,spotify", parsed.Query().Get("return"))
}

func TestStreams_SetCallbackUrl_DuplicateReturn_ErrsOut(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server")
	})
	defer func() { _ = c.Close() }()
	err := c.Streams().SetCallbackUrl("https://example.com/cb?return=apple_music",
		&SetCallbackUrlOptions{ReturnMetadata: []string{"spotify"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already contains")
}

func TestStreams_GetCallbackUrl(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
	})
	defer func() { _ = c.Close() }()
	url, err := c.Streams().GetCallbackUrl()
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/cb", url)
}

func TestStreams_Add(t *testing.T) {
	var seenURL, seenRadio, seenCallbacks string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenURL = r.FormValue("url")
		seenRadio = r.FormValue("radio_id")
		seenCallbacks = r.FormValue("callbacks")
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	err := c.Streams().Add(AddStreamRequest{
		URL: "twitch:foo", RadioID: 7, Callbacks: "before",
	})
	require.NoError(t, err)
	assert.Equal(t, "twitch:foo", seenURL)
	assert.Equal(t, "7", seenRadio)
	assert.Equal(t, "before", seenCallbacks)
}

func TestStreams_Delete_AndSetURL(t *testing.T) {
	var path string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.Streams().Delete(5))
	assert.Equal(t, "/deleteStream/", path)

	require.NoError(t, c.Streams().SetURL(5, "https://x"))
	assert.Equal(t, "/setStreamUrl/", path)
}

func TestStreams_List_ParsesArray(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[{"radio_id":1,"url":"twitch:a","stream_running":true}]}`))
	})
	defer func() { _ = c.Close() }()

	streams, err := c.Streams().List()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	assert.Equal(t, "twitch:a", streams[0].URL)
}

func TestStreams_List_EmptyResultIsEmptySlice(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	streams, err := c.Streams().List()
	require.NoError(t, err)
	assert.Empty(t, streams)
}

func TestStreams_DeriveLongpollCategory(t *testing.T) {
	c := NewClient("test")
	defer func() { _ = c.Close() }()
	cat := c.Streams().DeriveLongpollCategory(7)
	assert.Len(t, cat, 9)
	// Identical to top-level helper.
	assert.Equal(t, DeriveLongpollCategory("test", 7), cat)
}

func TestStreams_Longpoll_PreflightFailsOnNoCallbackURL(t *testing.T) {
	requests := 0
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/getCallbackUrl/" {
			_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":19,"error_message":"no callback url"}}`))
			return
		}
		t.Fatalf("unexpected path %s after preflight", r.URL.Path)
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := c.Streams().LongpollContext(ctx, "cat", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no callback URL")
	assert.Equal(t, 1, requests, "preflight only — must not start subscribing")
}

func TestStreams_Longpoll_PreflightSkippable(t *testing.T) {
	calls := 0
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/longpoll/", r.URL.Path, "should NOT preflight when SkipCallbackCheck=true")
		_, _ = w.Write([]byte(`{"timeout":"no events before timeout"}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	p, err := c.Streams().LongpollContext(ctx, "cat", &LongpollOptions{SkipCallbackCheck: true, Timeout: 50})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()
	// "timeout" responses are benign keep-alives — must NOT surface as events.
	select {
	case <-p.Matches:
		t.Fatal("keepalive must not produce a match")
	case <-p.Notifications:
		t.Fatal("keepalive must not produce a notification")
	case err := <-p.Errors:
		t.Fatalf("keepalive must not produce an error: %v", err)
	case <-ctx.Done():
		// expected
	}
}

func TestStreams_Longpoll_PreflightOK_DispatchesMatch(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			_, _ = w.Write([]byte(`{"status":"success","result":{"radio_id":7,"timestamp":"x","results":[{"artist":"A","title":"T","score":99}]}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := c.Streams().LongpollContext(ctx, "cat", nil)
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	select {
	case m := <-p.Matches:
		assert.Equal(t, "A", m.Song.Artist)
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-ctx.Done():
		t.Fatal("expected a match")
	}
}

func TestStreams_Longpoll_HTTP500_ClosesWithError(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`<html>uh oh</html>`))
		}
	})
	defer func() { _ = c.Close() }()

	// Reduce attempts to keep the test fast; retry policy on read class
	// would otherwise retry 500.
	c.maxAttempts = 1
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := c.Streams().LongpollContext(ctx, "cat", nil)
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	err = <-p.Errors
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSerialization) || errors.Is(err, ErrServer))
}

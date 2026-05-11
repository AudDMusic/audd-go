package audd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
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
		&SetCallbackUrlOptions{ReturnMetadata: "apple_music,spotify"}))
	parsed, _ := url.Parse(seenURL)
	assert.Equal(t, "apple_music,spotify", parsed.Query().Get("return"))
}

func TestStreams_SetCallbackUrl_DuplicateReturn_ErrsOut(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server")
	})
	defer func() { _ = c.Close() }()
	err := c.Streams().SetCallbackUrl("https://example.com/cb?return=apple_music",
		&SetCallbackUrlOptions{ReturnMetadata: "spotify"})
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

func TestStreams_LongpollByRadioID_DerivesCategoryAndOpensSubscription(t *testing.T) {
	var seenCategory string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			seenCategory = r.URL.Query().Get("category")
			_, _ = w.Write([]byte(`{"status":"success","result":{"radio_id":42,"timestamp":"x","results":[{"artist":"A","title":"T","score":99}]}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer func() { _ = c.Close() }()

	p, err := c.Streams().LongpollByRadioID(42, nil)
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
	// The category sent on the wire must match what DeriveLongpollCategory
	// produces for the mock client's token + radio_id.
	expected := DeriveLongpollCategory("test", 42)
	assert.Equal(t, expected, seenCategory)
}

func TestStreams_LongpollByRadioIDContext_HonorsCancellation(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			_, _ = w.Write([]byte(`{"timeout":"no events before timeout"}`))
		}
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	p, err := c.Streams().LongpollByRadioIDContext(ctx, 42, &LongpollOptions{Timeout: 50})
	require.NoError(t, err)
	cancel()

	// All channels must close after ctx is cancelled.
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	matchClosed, notifClosed, errsClosed := false, false, false
	for !(matchClosed && notifClosed && errsClosed) {
		select {
		case _, ok := <-p.Matches:
			if !ok {
				matchClosed = true
			}
		case _, ok := <-p.Notifications:
			if !ok {
				notifClosed = true
			}
		case _, ok := <-p.Errors:
			if !ok {
				errsClosed = true
			}
		case <-deadline.C:
			t.Fatalf("channels did not all close on ctx cancel (matches=%v notifs=%v errs=%v)",
				matchClosed, notifClosed, errsClosed)
		}
	}
}

func TestStreams_LongpollByRadioID_SameCategoryAsTwoStepForm(t *testing.T) {
	// Both paths must hit the server with the same category string. We
	// run two subscriptions sequentially against the same mock and compare
	// the captured query parameters. The longpoll goroutine may produce
	// multiple requests before Close() takes effect; we just require every
	// captured category to be identical.
	var (
		mu         sync.Mutex
		categories []string
	)
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/longpoll/":
			mu.Lock()
			categories = append(categories, r.URL.Query().Get("category"))
			mu.Unlock()
			_, _ = w.Write([]byte(`{"status":"success","result":{"radio_id":42,"timestamp":"x","results":[{"artist":"A","title":"T","score":99}]}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer func() { _ = c.Close() }()

	// One-step path.
	p1, err := c.Streams().LongpollByRadioID(42, nil)
	require.NoError(t, err)
	select {
	case <-p1.Matches:
	case err := <-p1.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a match for one-step path")
	}
	_ = p1.Close()

	// Drain p1's channels so the goroutine has fully exited before we
	// snapshot the captured category list.
	drainPoll(p1)
	mu.Lock()
	oneStepCount := len(categories)
	mu.Unlock()

	// Two-step path: explicit derive + Longpoll(category, ...).
	cat := DeriveLongpollCategory("test", 42)
	p2, err := c.Streams().Longpoll(cat, nil)
	require.NoError(t, err)
	select {
	case <-p2.Matches:
	case err := <-p2.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a match for two-step path")
	}
	_ = p2.Close()
	drainPoll(p2)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, oneStepCount, 1, "one-step path must have hit /longpoll/ at least once")
	require.Greater(t, len(categories), oneStepCount, "two-step path must have hit /longpoll/ at least once")
	for i, got := range categories {
		assert.Equal(t, cat, got, "category mismatch at index %d", i)
	}
}

// drainPoll consumes the three channels until they all close. Used after
// Close() to make sure the background goroutine has fully exited and is no
// longer mutating shared test state.
func drainPoll(p *LongpollPoll) {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	matchClosed, notifClosed, errsClosed := false, false, false
	for !(matchClosed && notifClosed && errsClosed) {
		select {
		case _, ok := <-p.Matches:
			if !ok {
				matchClosed = true
			}
		case _, ok := <-p.Notifications:
			if !ok {
				notifClosed = true
			}
		case _, ok := <-p.Errors:
			if !ok {
				errsClosed = true
			}
		case <-timer.C:
			return
		}
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

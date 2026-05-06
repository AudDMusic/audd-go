package audd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the no-context convenience wrappers. Every public method on the
// SDK has both a `Foo(args)` form (defaults to context.Background()) and a
// `FooContext(ctx, args)` form. These tests verify the wrappers reach the
// expected endpoint and that the explicit-context counterparts still work.

func TestRecognizeDefaultsToBackgroundContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)
	c := mkPolishClient("t", srv.URL)
	if _, err := c.Recognize("https://x.mp3", nil); err != nil {
		t.Fatalf("Recognize (no ctx): %v", err)
	}
}

func TestRecognizeContextStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)
	c := mkPolishClient("t", srv.URL)
	if _, err := c.RecognizeContext(context.Background(), "https://x.mp3", nil); err != nil {
		t.Fatalf("RecognizeContext: %v", err)
	}
}

func TestStreams_NonContextWrappers_HitExpectedPaths(t *testing.T) {
	var seenPaths []string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.Path)
		switch r.URL.Path {
		case "/getCallbackUrl/":
			_, _ = w.Write([]byte(`{"status":"success","result":"https://example.com/cb"}`))
		case "/getStreams/":
			_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","result":true}`))
		}
	})
	defer func() { _ = c.Close() }()

	require.NoError(t, c.Streams().SetCallbackUrl("https://example.com/cb", nil))
	_, err := c.Streams().GetCallbackUrl()
	require.NoError(t, err)
	require.NoError(t, c.Streams().Add(AddStreamRequest{URL: "twitch:foo", RadioID: 1}))
	require.NoError(t, c.Streams().SetURL(1, "twitch:bar"))
	_, err = c.Streams().List()
	require.NoError(t, err)
	require.NoError(t, c.Streams().Delete(1))

	assert.Equal(t, []string{
		"/setCallbackUrl/",
		"/getCallbackUrl/",
		"/addStream/",
		"/setStreamUrl/",
		"/getStreams/",
		"/deleteStream/",
	}, seenPaths)
}

func TestAdvanced_NonContextWrappers_HitExpectedPaths(t *testing.T) {
	var seenPath string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Advanced().FindLyrics("hello")
	require.NoError(t, err)
	assert.Equal(t, "/findLyrics/", seenPath)

	_, err = c.Advanced().RawRequest("someMethod", map[string]string{"k": "v"})
	require.NoError(t, err)
	assert.Equal(t, "/someMethod/", seenPath)
}

func TestLongpollConsumer_NonContextWrapper_DefaultsToBackground(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"timeout":"no events before timeout"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewLongpollConsumer("cat", LongpollConsumerWithMaxAttempts(1))
	defer func() { _ = c.Close() }()
	c.httpc.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}

	p := c.Iterate(&LongpollConsumerOptions{Timeout: 1})
	defer func() { _ = p.Close() }()
	// Keepalive body is silently absorbed — confirm by waiting and
	// checking no events fire within a short window.
	select {
	case <-p.Matches:
		t.Fatal("keepalive must not produce a match")
	case <-p.Notifications:
		t.Fatal("keepalive must not produce a notification")
	case err := <-p.Errors:
		t.Fatalf("keepalive must not produce an error: %v", err)
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestCustomCatalog_NonContextWrapper_HitsUploadEndpoint(t *testing.T) {
	var seenPath string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()
	require.NoError(t, c.CustomCatalog().Add(7, []byte("buf")))
	assert.Equal(t, "/upload/", seenPath)
}

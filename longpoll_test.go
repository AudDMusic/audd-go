package audd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConsumer(t *testing.T, h http.HandlerFunc) (*LongpollConsumer, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewLongpollConsumer("cat", LongpollConsumerWithMaxAttempts(1))
	c.httpc.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	return c, srv
}

func TestLongpollConsumer_HappyPath(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/longpoll/", r.URL.Path)
		assert.Equal(t, "cat", r.URL.Query().Get("category"))
		assert.Empty(t, r.URL.Query().Get("api_token"), "tokenless: must not send api_token")
		_, _ = w.Write([]byte(`{"timeout":"no events before timeout"}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ch := c.IterateContext(ctx, nil)
	select {
	case ev := <-ch:
		require.NoError(t, ev.Err)
		assert.NotNil(t, ev.Body)
	case <-ctx.Done():
		t.Fatal("expected an event")
	}
}

func TestLongpollConsumer_HTTP500_StopsWithError(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("server died"))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch := c.IterateContext(ctx, nil)
	ev, ok := <-ch
	require.True(t, ok)
	require.Error(t, ev.Err)
	assert.ErrorIs(t, ev.Err, ErrServer)

	// Channel should be closed after a terminal error.
	_, more := <-ch
	assert.False(t, more, "channel must close after terminal error")
}

func TestLongpollConsumer_HTTP401_StopsWithError(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ev := <-c.IterateContext(ctx, nil)
	require.Error(t, ev.Err)
	assert.ErrorIs(t, ev.Err, ErrServer)
}

func TestLongpollConsumer_BadJSON_StopsWithSerializationError(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ev := <-c.IterateContext(ctx, nil)
	require.Error(t, ev.Err)
	assert.ErrorIs(t, ev.Err, ErrSerialization)
}

func TestLongpollConsumer_RetriesOn5xx_AndSucceeds(t *testing.T) {
	calls := 0
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"timeout":"x"}`))
	})
	c.maxAttempts = 3
	c.backoffFactor = time.Millisecond
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev := <-c.IterateContext(ctx, nil)
	assert.NoError(t, ev.Err)
	assert.GreaterOrEqual(t, calls, 2)
}

func TestLongpollConsumer_ContextCancelClosesChannel(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"timeout":"x"}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	ch := c.IterateContext(ctx, nil)
	cancel()

	// Drain until close — must terminate.
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
loop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break loop
			}
		case <-deadline.C:
			t.Fatal("channel did not close on ctx cancel")
		}
	}
}

func TestLongpollConsumer_Close_Idempotent(t *testing.T) {
	c := NewLongpollConsumer("x")
	assert.NoError(t, c.Close())
	assert.NoError(t, c.Close())
}

func TestLongpollConsumer_ConnectionError_ClosesWithErr(t *testing.T) {
	// Point at a closed listener: a connect error.
	c := NewLongpollConsumer("cat", LongpollConsumerWithMaxAttempts(1), LongpollConsumerWithBackoffFactor(time.Millisecond))
	c.httpc.hc = &http.Client{Transport: rewriteTransport{base: "http://127.0.0.1:1"}, Timeout: 200 * time.Millisecond}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev := <-c.IterateContext(ctx, nil)
	require.Error(t, ev.Err)
	var connErr *AuddConnectionError
	assert.True(t, errors.As(ev.Err, &connErr) || errors.Is(ev.Err, ErrConnection))
}

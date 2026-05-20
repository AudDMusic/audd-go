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

// awaitErr drains a longpoll until it produces an error, a match, a
// notification, or the deadline fires.
type pollOutcome struct {
	match *StreamCallbackMatch
	notif *StreamCallbackNotification
	err   error
}

func awaitOutcome(t *testing.T, p *LongpollPoll, deadline time.Duration) pollOutcome {
	t.Helper()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case m, ok := <-p.Matches:
			if !ok {
				continue
			}
			return pollOutcome{match: &m}
		case n, ok := <-p.Notifications:
			if !ok {
				continue
			}
			return pollOutcome{notif: &n}
		case err, ok := <-p.Errors:
			if !ok {
				return pollOutcome{}
			}
			return pollOutcome{err: err}
		case <-timer.C:
			t.Fatal("longpoll produced no outcome before deadline")
			return pollOutcome{}
		}
	}
}

func TestLongpollConsumer_TimeoutNoEvents_KeptAlive(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/longpoll/", r.URL.Path)
		assert.Equal(t, "cat", r.URL.Query().Get("category"))
		assert.Empty(t, r.URL.Query().Get("api_token"), "tokenless: must not send api_token")
		_, _ = w.Write([]byte(`{"timeout":"no events before timeout"}`))
	})
	defer func() { _ = c.Close() }()

	// Keepalive responses are NOT consumer-facing. Run for a short window
	// and confirm no spurious match/notification/error fires.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	p := c.IterateContext(ctx, nil)
	defer func() { _ = p.Close() }()

	select {
	case <-p.Matches:
		t.Fatal("keepalive must not produce a match")
	case <-p.Notifications:
		t.Fatal("keepalive must not produce a notification")
	case err := <-p.Errors:
		t.Fatalf("keepalive must not produce an error: %v", err)
	case <-ctx.Done():
		// expected — no consumer-facing events fired
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
	p := c.IterateContext(ctx, nil)
	defer func() { _ = p.Close() }()

	out := awaitOutcome(t, p, 2*time.Second)
	require.Error(t, out.err)
	assert.ErrorIs(t, out.err, ErrServer)

	// After a terminal error, all channels close.
	_, more := <-p.Errors
	assert.False(t, more, "errors channel must close after terminal error")
}

func TestLongpollConsumer_HTTP401_StopsWithError(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out := awaitOutcome(t, c.IterateContext(ctx, nil), 2*time.Second)
	require.Error(t, out.err)
	assert.ErrorIs(t, out.err, ErrServer)
}

func TestLongpollConsumer_BadJSON_StopsWithSerializationError(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out := awaitOutcome(t, c.IterateContext(ctx, nil), 2*time.Second)
	require.Error(t, out.err)
	assert.ErrorIs(t, out.err, ErrSerialization)
}

func TestLongpollConsumer_MatchYieldsTypedSong(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":{"radio_id":7,"timestamp":"x","results":[{"artist":"A","title":"T","score":99}]}}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p := c.IterateContext(ctx, nil)
	defer func() { _ = p.Close() }()

	select {
	case m := <-p.Matches:
		assert.Equal(t, int64(7), m.RadioID)
		assert.Equal(t, "A", m.Song.Artist)
		assert.Equal(t, "T", m.Song.Title)
		assert.Equal(t, 99, m.Song.Score)
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a match")
	}
}

func TestLongpollConsumer_NotificationYieldsTyped(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"-","notification":{"radio_id":3,"stream_running":false,"notification_code":650,"notification_message":"can't connect"},"time":1587939136}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p := c.IterateContext(ctx, nil)
	defer func() { _ = p.Close() }()

	select {
	case n := <-p.Notifications:
		assert.Equal(t, 3, n.RadioID)
		assert.Equal(t, 650, n.NotificationCode)
		assert.Equal(t, 1587939136, n.Time)
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification")
	}
}

func TestLongpollConsumer_ContextCancelClosesChannels(t *testing.T) {
	c, _ := newConsumer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"-","notification":{"radio_id":1,"notification_code":1,"notification_message":"x"}}`))
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	p := c.IterateContext(ctx, nil)
	cancel()

	// Drain until all channels close — must terminate.
	deadline := time.NewTimer(time.Second)
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

func TestLongpollConsumer_Close_Idempotent(t *testing.T) {
	c := NewLongpollConsumer("x")
	assert.NoError(t, c.Close())
	assert.NoError(t, c.Close())
}

func TestLongpollConsumer_ConnectionError_ClosesWithErr(t *testing.T) {
	c := NewLongpollConsumer("cat", LongpollConsumerWithMaxAttempts(1), LongpollConsumerWithBackoffFactor(time.Millisecond))
	c.httpc.hc = &http.Client{Transport: rewriteTransport{base: "http://127.0.0.1:1"}, Timeout: 200 * time.Millisecond}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out := awaitOutcome(t, c.IterateContext(ctx, nil), 2*time.Second)
	require.Error(t, out.err)
	var connErr *AudDConnectionError
	assert.True(t, errors.As(out.err, &connErr) || errors.Is(out.err, ErrConnection))
}

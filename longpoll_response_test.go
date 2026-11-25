package audd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The longpoll fixtures in testdata/ are copies of the shared audd-openapi
// fixtures of the same names.
func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(b)
}

// lpScript serves a fixed sequence of longpoll bodies, one per request, and
// records every request. Once the script runs out, a request is held open
// (like a real long poll with no events) until the client gives up.
type lpScript struct {
	mu     sync.Mutex
	bodies []string
	reqs   []*http.Request
	seen   chan struct{}
	done   chan struct{}
}

func newLPScript(bodies ...string) *lpScript {
	return &lpScript{bodies: bodies, seen: make(chan struct{}, 64), done: make(chan struct{})}
}

func (s *lpScript) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	i := len(s.reqs)
	s.reqs = append(s.reqs, r)
	s.mu.Unlock()
	select {
	case s.seen <- struct{}{}:
	default:
	}
	if i < len(s.bodies) {
		_, _ = w.Write([]byte(s.bodies[i]))
		return
	}
	select {
	case <-r.Context().Done():
	case <-s.done:
	}
}

func (s *lpScript) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reqs)
}

func (s *lpScript) query(i int) url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reqs[i].URL.Query()
}

// waitForRequests blocks until at least n requests arrived.
func (s *lpScript) waitForRequests(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for s.count() < n {
		select {
		case <-s.seen:
		case <-deadline:
			t.Fatalf("expected %d longpoll requests, got %d", n, s.count())
		}
	}
}

// newScriptedConsumer starts a tokenless consumer against the script.
func newScriptedConsumer(t *testing.T, bodies ...string) (*LongpollConsumer, *lpScript) {
	t.Helper()
	s := newLPScript(bodies...)
	srv := httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(s.done) })
	c := NewLongpollConsumer("92b1cc7f0", LongpollConsumerWithMaxAttempts(1))
	c.httpc.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	t.Cleanup(func() { _ = c.Close() })
	return c, s
}

// newScriptedClient starts an authenticated client whose /longpoll/ is the
// script.
func newScriptedClient(t *testing.T, bodies ...string) (*Client, *lpScript) {
	t.Helper()
	s := newLPScript(bodies...)
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/longpoll/" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		s.handle(w, r)
	})
	t.Cleanup(func() { close(s.done) })
	return c, s
}

func recvMatch(t *testing.T, p *LongpollPoll) StreamCallbackMatch {
	t.Helper()
	select {
	case m, ok := <-p.Matches:
		require.True(t, ok, "Matches closed before a match arrived")
		return m
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("expected a match")
	}
	return StreamCallbackMatch{}
}

func TestLongpoll_EventsFixture_DeliversSongsInOrderAndAdvancesSinceTime(t *testing.T) {
	c, s := newScriptedConsumer(t, readTestdata(t, "longpoll_events.json"))
	p := c.Iterate(nil)
	defer func() { _ = p.Close() }()

	first := recvMatch(t, p)
	assert.Equal(t, int64(7), first.RadioID)
	assert.Equal(t, "Alan Walker, A$AP Rocky", first.Song.Artist)
	assert.Equal(t, "Live Fast (PUBGM)", first.Song.Title)
	assert.Equal(t, 111, first.PlayLength)
	// RawResponse is the event's callback body, not the whole events response.
	assert.Contains(t, string(first.RawResponse), `"play_length": 111`)
	assert.NotContains(t, string(first.RawResponse), `"events"`)
	assert.NotContains(t, string(first.RawResponse), "Tears For Fears")

	second := recvMatch(t, p)
	assert.Equal(t, int64(7), second.RadioID)
	assert.Equal(t, "Tears For Fears", second.Song.Artist)
	assert.Equal(t, "Everybody Wants To Rule The World", second.Song.Title)

	s.waitForRequests(t, 2)
	assert.Empty(t, s.query(0).Get("since_time"), "first request starts from now")
	assert.Equal(t, "1795547548112", s.query(1).Get("since_time"))
	assert.Equal(t, "92b1cc7f0", s.query(1).Get("category"))
	assert.Equal(t, "50", s.query(1).Get("timeout"))
}

func TestLongpoll_TimeoutFixture_DeliversNothingAndSendsItsTimestamp(t *testing.T) {
	c, s := newScriptedConsumer(t, readTestdata(t, "longpoll_no_events.json"))
	p := c.Iterate(&LongpollConsumerOptions{SinceTime: 1777901200000})
	defer func() { _ = p.Close() }()

	s.waitForRequests(t, 2)
	assert.Equal(t, "1777901200000", s.query(0).Get("since_time"))
	assert.Equal(t, "1777901270049", s.query(1).Get("since_time"))
	select {
	case m := <-p.Matches:
		t.Fatalf("timeout response must not produce a match: %+v", m)
	case n := <-p.Notifications:
		t.Fatalf("timeout response must not produce a notification: %+v", n)
	case err := <-p.Errors:
		t.Fatalf("timeout response must not produce an error: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLongpoll_ErrorFixture_EndsPollWithInvalidRequestAfterOneRequest(t *testing.T) {
	c, s := newScriptedConsumer(t, readTestdata(t, "longpoll_error.json"), readTestdata(t, "longpoll_error.json"))
	p := c.Iterate(&LongpollConsumerOptions{Timeout: 180})
	defer func() { _ = p.Close() }()

	var err error
	select {
	case err = <-p.Errors:
	case <-time.After(3 * time.Second):
		t.Fatal("expected the error body to reach Errors")
	}
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
	assert.NotErrorIs(t, err, ErrServer)
	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "Invalid timeout arg.  Must be 1-120.", apiErr.Message)
	assert.Equal(t, 200, apiErr.HTTPStatus)

	// The poll is over: every channel closes.
	drainPoll(p)
	_, open := <-p.Errors
	assert.False(t, open)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 1, s.count(), "an error body must not be requested again")
	assert.Equal(t, "180", s.query(0).Get("timeout"))
}

func TestLongpoll_OldTopLevelShape_StillDelivers(t *testing.T) {
	c, s := newScriptedConsumer(t,
		`{"status":"success","timestamp":1795547000000,"result":{"radio_id":3,"timestamp":"2026-11-24 19:00:00","results":[{"artist":"A","title":"T","score":90}]}}`,
		`{"status":"-","notification":{"radio_id":3,"stream_running":false,"notification_code":650,"notification_message":"can't connect"},"time":1587939136}`,
	)
	p := c.Iterate(nil)
	defer func() { _ = p.Close() }()

	m := recvMatch(t, p)
	assert.Equal(t, int64(3), m.RadioID)
	assert.Equal(t, "A", m.Song.Artist)

	select {
	case n := <-p.Notifications:
		assert.Equal(t, 650, n.NotificationCode)
		assert.Equal(t, 3, n.RadioID)
	case err := <-p.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("expected a notification")
	}
	s.waitForRequests(t, 2)
	assert.Equal(t, "1795547000000", s.query(1).Get("since_time"), "a top-level timestamp is still honoured")
}

func TestLongpoll_EventEdgeCases_SkippedWithoutEndingThePoll(t *testing.T) {
	body := `{"events":[
		{"timestamp":1795547000001,"category":"c","data":{"status":"success","result":{"radio_id":1,"results":[]}}},
		{"timestamp":1795547000002,"category":"c","data":"not a callback body"},
		{"timestamp":1795547000003,"category":"c","data":{"unexpected":true}},
		{"timestamp":"1795547000004","category":"c","data":"{\"status\":\"success\",\"result\":{\"radio_id\":2,\"results\":[{\"artist\":\"String\",\"title\":\"Data\"}]}}"},
		{"timestamp":1795547000005,"category":"c","data":{"status":"-","notification":{"radio_id":2,"notification_code":650,"notification_message":"m"}}},
		"not an event",
		{"timestamp":1795547000006,"category":"c","data":{"status":"success","result":{"radio_id":2,"results":[{"artist":"Last","title":"One"}]}}}
	]}`
	c, s := newScriptedConsumer(t, body)
	p := c.Iterate(nil)
	defer func() { _ = p.Close() }()

	m := recvMatch(t, p)
	assert.Equal(t, "String", m.Song.Artist, "data sent as a JSON string is decoded")
	assert.Equal(t, int64(2), m.RadioID)

	m = recvMatch(t, p)
	assert.Equal(t, "Last", m.Song.Artist)

	select {
	case n := <-p.Notifications:
		assert.Equal(t, 650, n.NotificationCode)
	case <-time.After(time.Second):
		t.Fatal("expected the notification event")
	}
	s.waitForRequests(t, 2)
	assert.Equal(t, "1795547000006", s.query(1).Get("since_time"))
}

func TestLongpoll_NotificationsDoNotStallAMatchesOnlyReader(t *testing.T) {
	var events []string
	for i := 0; i < 40; i++ {
		events = append(events, `{"timestamp":17955470000`+twoDigits(i)+`,"data":{"status":"-","notification":{"radio_id":1,"notification_code":`+strconv.Itoa(i)+`,"notification_message":"m"}}}`)
	}
	events = append(events, `{"timestamp":1795547000099,"data":{"status":"success","result":{"radio_id":1,"results":[{"artist":"After","title":"Notifications"}]}}}`)
	c, _ := newScriptedConsumer(t, `{"events":[`+strings.Join(events, ",")+`]}`)
	p := c.Iterate(nil)
	defer func() { _ = p.Close() }()

	// Read only Matches.
	m := recvMatch(t, p)
	assert.Equal(t, "After", m.Song.Artist)

	// The buffer kept the newest notifications.
	var codes []int
	for len(codes) < longpollNotificationBuffer {
		select {
		case n := <-p.Notifications:
			codes = append(codes, n.NotificationCode)
		case <-time.After(time.Second):
			t.Fatalf("expected %d buffered notifications, got %d", longpollNotificationBuffer, len(codes))
		}
	}
	assert.Equal(t, 39, codes[len(codes)-1])
	assert.Equal(t, 40-longpollNotificationBuffer, codes[0])
}

func twoDigits(i int) string {
	return string([]byte{byte('0' + i/10), byte('0' + i%10)})
}

func TestLongpoll_CloseAbortsAWaitingRequest(t *testing.T) {
	c, s := newScriptedConsumer(t) // every request is held open
	p := c.Iterate(nil)
	s.waitForRequests(t, 1)

	start := time.Now()
	require.NoError(t, p.Close())
	deadline := time.After(2 * time.Second)
	matchClosed, notifClosed, errsClosed := false, false, false
	for !(matchClosed && notifClosed && errsClosed) {
		select {
		case _, ok := <-p.Matches:
			matchClosed = !ok
		case _, ok := <-p.Notifications:
			notifClosed = !ok
		case err, ok := <-p.Errors:
			if ok {
				t.Fatalf("Close must not send an error: %v", err)
			}
			errsClosed = true
		case <-deadline:
			t.Fatal("Close did not stop the waiting request")
		}
	}
	assert.Less(t, time.Since(start), time.Second)
}

func TestStreams_Longpoll_EventsAndNoAPIToken(t *testing.T) {
	c, s := newScriptedClient(t, readTestdata(t, "longpoll_events.json"))
	defer func() { _ = c.Close() }()
	p, err := c.Streams().LongpollContext(context.Background(), "92b1cc7f0", &LongpollOptions{SkipCallbackCheck: true})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	assert.Equal(t, "Alan Walker, A$AP Rocky", recvMatch(t, p).Song.Artist)
	assert.Equal(t, "Tears For Fears", recvMatch(t, p).Song.Artist)
	s.waitForRequests(t, 2)
	assert.Equal(t, "1795547548112", s.query(1).Get("since_time"))

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.reqs {
		assert.NotContains(t, r.URL.RawQuery, "api_token", "request %d", i)
		assert.NotContains(t, r.URL.RawQuery, "test-token", "request %d", i)
		for name, values := range r.Header {
			for _, v := range values {
				assert.NotContains(t, v, "test-token", "request %d header %s", i, name)
			}
		}
	}
}

func TestStreams_Longpoll_ErrorBodyEndsPoll(t *testing.T) {
	c, s := newScriptedClient(t, readTestdata(t, "longpoll_error.json"))
	defer func() { _ = c.Close() }()
	p, err := c.Streams().LongpollContext(context.Background(), "cat", &LongpollOptions{SkipCallbackCheck: true, Timeout: 500})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	select {
	case err := <-p.Errors:
		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Contains(t, err.Error(), "Invalid timeout arg.  Must be 1-120.")
	case <-time.After(3 * time.Second):
		t.Fatal("expected the error body to reach Errors")
	}
	drainPoll(p)
	assert.Equal(t, 1, s.count())
}

func TestStreams_Longpoll_AudDErrorBlockMapsByCode(t *testing.T) {
	c, _ := newScriptedClient(t, `{"status":"error","error":{"error_code":900,"error_message":"Wrong API token"}}`)
	defer func() { _ = c.Close() }()
	p, err := c.Streams().LongpollContext(context.Background(), "cat", &LongpollOptions{SkipCallbackCheck: true})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	select {
	case err := <-p.Errors:
		assert.ErrorIs(t, err, ErrAuthentication)
	case <-time.After(3 * time.Second):
		t.Fatal("expected an error")
	}
}

// failingTransport fails every request the way a dropped connection does.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestStreams_Longpoll_ConnectionErrorTextHasNoToken(t *testing.T) {
	c := NewClient("SECRET-TOKEN-XYZ", WithMaxAttempts(1))
	defer func() { _ = c.Close() }()
	c.longpollHTTP.hc = &http.Client{Transport: failingTransport{}}

	p, err := c.Streams().LongpollContext(context.Background(), "3232c7fe9", &LongpollOptions{SkipCallbackCheck: true, Timeout: 1})
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	select {
	case err := <-p.Errors:
		require.ErrorIs(t, err, ErrConnection)
		assert.NotContains(t, err.Error(), "SECRET-TOKEN-XYZ")
		assert.NotContains(t, err.Error(), "api_token")
		assert.Contains(t, err.Error(), "https://api.audd.io/longpoll/")
	case <-time.After(3 * time.Second):
		t.Fatal("expected a connection error")
	}
}

func TestLongpoll_EventsWithoutTimestamps_DoNotLoop(t *testing.T) {
	body := `{"events":[{"data":{"status":"success","result":{"radio_id":1,"results":[{"artist":"A","title":"T"}]}}}]}`
	c, s := newScriptedConsumer(t, body, body, body, body, body)
	p := c.Iterate(nil)
	defer func() { _ = p.Close() }()

	assert.Equal(t, "A", recvMatch(t, p).Song.Artist)
	// Without a timestamp there is no newer since_time to ask for, so the
	// next request waits instead of fetching the same events at once.
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 1, s.count())
}

func TestParseLongpollResponse_Shapes(t *testing.T) {
	page := func(body string) longpollPage {
		return parseLongpollResponse(&httpResponse{HTTPStatus: 200, RawBody: []byte(body)})
	}
	p := page(`{"timeout":"no events before timeout","timestamp":"1777901270049"}`)
	assert.True(t, p.hasSince)
	assert.Equal(t, int64(1777901270049), p.since, "a numeric-string timestamp is accepted")

	p = page(`{"timeout":"no events before timeout"}`)
	assert.False(t, p.hasSince)
	assert.Empty(t, p.events)
	assert.NoError(t, p.err)

	p = page(`{"events":[{"timestamp":5,"data":null},{"timestamp":9,"data":{"status":"success","result":{"results":[{"artist":"x"}]}}},{"timestamp":7}]}`)
	assert.Equal(t, int64(9), p.since, "the largest event timestamp wins")
	assert.Len(t, p.events, 1)

	p = page(`{"events":"nope"}`)
	assert.False(t, p.hasSince)
	assert.Empty(t, p.events)

	p = page(`{"error":""}`)
	assert.ErrorIs(t, p.err, ErrInvalidRequest)

	p = page(`{"error":null,"timeout":"no events before timeout","timestamp":3}`)
	assert.NoError(t, p.err)
	assert.Equal(t, int64(3), p.since)

	// The top-level callback shape delivers a recognition with no songs (a
	// zero Song), as it always has; inside `events` such a recognition is
	// skipped.
	p = page(`{"status":"success","result":{"radio_id":4,"results":[]}}`)
	require.Len(t, p.events, 1)
	assert.Equal(t, int64(4), p.events[0].match.RadioID)
	p = page(`{"events":[{"timestamp":8,"data":{"status":"success","result":{"radio_id":4,"results":[]}}}]}`)
	assert.Empty(t, p.events)
	assert.Equal(t, int64(8), p.since)
}

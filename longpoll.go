package audd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// longpollURL is the AudD longpoll endpoint. Tokenless — caller authorizes
// the subscription via the category alone.
const longpollURL = "https://api.audd.io/longpoll/"

// defaultLongpollTimeout is the poll timeout, in seconds, used when the
// caller leaves Timeout at 0. The server accepts 1 to 120.
const defaultLongpollTimeout = 50

// A response with nothing new in it (no event, no new timestamp) is followed
// by a pause that doubles from longpollIdleBackoffMin up to
// longpollIdleBackoffMax.
const (
	longpollIdleBackoffMin = time.Second
	longpollIdleBackoffMax = 30 * time.Second
)

// longpollNotificationBuffer is how many unread notifications a poll holds
// before it drops the oldest one.
const longpollNotificationBuffer = 16

// LongpollConsumerOption configures a LongpollConsumer.
type LongpollConsumerOption func(*LongpollConsumer)

// LongpollConsumerWithHTTPClient injects a caller-managed *http.Client.
// Leave its Timeout at 0: each longpoll request already gets a deadline sized
// to the poll timeout plus a margin, and a shorter http.Client.Timeout would
// cut every wait short.
func LongpollConsumerWithHTTPClient(hc *http.Client) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.userHTTPClient = hc }
}

// LongpollConsumerWithMaxAttempts overrides the number of attempts per
// longpoll request (default 3) for HTTP 408/429/5xx and network failures.
// 1 disables the retries, so the first such failure ends the poll.
func LongpollConsumerWithMaxAttempts(n int) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.maxAttempts = n }
}

// LongpollConsumerWithBackoffFactor overrides the default initial-backoff.
func LongpollConsumerWithBackoffFactor(d time.Duration) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.backoffFactor = d }
}

// LongpollConsumer is the tokenless long-polling consumer. Use it from
// browser/widget/extension contexts where you only have a category (and
// no api_token).
//
// Implements io.Closer (Close releases the owned HTTP transport).
//
// Each poll it starts (Iterate / IterateContext) behaves as described on
// LongpollPoll:
//   - Events are delivered in order, and the next request asks only for
//     events newer than the newest one.
//   - An error body from the server (for example a timeout outside 1-120)
//     ends the poll with an *AudDAPIError matching ErrInvalidRequest.
//   - HTTP 408/429/5xx and network failures are retried with backoff up to
//     MaxAttempts per request; after that the poll ends with an
//     *AudDAPIError matching ErrServer or an *AudDConnectionError.
//   - A response with nothing new in it (an unknown shape, an empty body,
//     non-JSON) is followed by a pause that grows from 1 to 30 seconds.
type LongpollConsumer struct {
	category       string
	userHTTPClient *http.Client
	maxAttempts    int
	backoffFactor  time.Duration

	httpc *httpClient
}

// NewLongpollConsumer builds a tokenless consumer for the given category.
func NewLongpollConsumer(category string, opts ...LongpollConsumerOption) *LongpollConsumer {
	c := &LongpollConsumer{
		category:      category,
		maxAttempts:   3,
		backoffFactor: 500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	// No client-level timeout: each longpoll request gets a per-request
	// deadline sized to the poll timeout plus a margin.
	c.httpc = newHTTPClient("", 0, c.userHTTPClient)
	return c
}

// Close releases the owned HTTP transport. Idempotent.
func (c *LongpollConsumer) Close() error {
	if c.httpc != nil {
		_ = c.httpc.Close()
	}
	return nil
}

// LongpollConsumerOptions controls a single Iterate call.
type LongpollConsumerOptions struct {
	// SinceTime is a server timestamp in milliseconds since the Unix epoch,
	// as carried by an earlier longpoll response: only events newer than it
	// are delivered. 0 means "start from now".
	SinceTime int
	// Timeout is how long, in seconds, the server holds each longpoll request
	// open while waiting for events: 1 to 120. 0 means 50.
	Timeout int
}

// Iterate is the no-context convenience wrapper around IterateContext.
// Defaults to context.Background(). Callers that want to stop the background
// goroutine without draining the channels should use IterateContext with a
// cancellable context (or call Close on the returned poll).
func (c *LongpollConsumer) Iterate(opts *LongpollConsumerOptions) *LongpollPoll {
	return c.IterateContext(context.Background(), opts)
}

// IterateContext returns a LongpollPoll whose Matches / Notifications /
// Errors channels are filled by a background goroutine. The poll terminates
// when ctx is cancelled, Close() is called, or a terminal error fires.
//
// Tokenless: no api_token is sent. The category alone authorizes the
// subscription. The user/server who derived the category is responsible for
// ensuring a callback URL is configured on their account (we can't
// preflight that without a token).
func (c *LongpollConsumer) IterateContext(ctx context.Context, opts *LongpollConsumerOptions) *LongpollPoll {
	if opts == nil {
		opts = &LongpollConsumerOptions{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultLongpollTimeout
	}
	policy := RetryPolicy{
		Class:         RetryClassRead,
		MaxAttempts:   c.maxAttempts,
		BackoffFactor: c.backoffFactor,
	}
	return startLongpoll(ctx, longpollSource{
		fetch: func(ctx context.Context, params map[string]string) (*httpResponse, error) {
			return retryDo(ctx, policy, func() (*httpResponse, error) {
				// Size the request deadline to the poll timeout + margin so
				// poll timeouts above 60s aren't cut short by the transport.
				reqCtx, cancel := context.WithTimeout(ctx, longpollRequestTimeout(timeout))
				defer cancel()
				return c.httpc.getNoToken(reqCtx, longpollURL, params)
			})
		},
		category:  c.category,
		sinceTime: int64(opts.SinceTime),
		timeout:   timeout,
	})
}

func stringOrJSON(r *httpResponse) any {
	if r.JSONBody != nil {
		return r.JSONBody
	}
	return string(r.RawBody)
}

// longpollSource abstracts the HTTP fetch so authenticated and tokenless
// consumers share the same dispatch loop.
type longpollSource struct {
	fetch     func(ctx context.Context, params map[string]string) (*httpResponse, error)
	category  string
	sinceTime int64 // milliseconds
	timeout   int   // seconds
}

// startLongpoll creates a poll's channels and starts its goroutine.
func startLongpoll(ctx context.Context, src longpollSource) *LongpollPoll {
	pollCtx, cancel := context.WithCancel(ctx)
	matches := make(chan StreamCallbackMatch)
	notifs := make(chan StreamCallbackNotification, longpollNotificationBuffer)
	errs := make(chan error, 1)
	go runLongpoll(pollCtx, src, matches, notifs, errs)
	return &LongpollPoll{
		Matches:       matches,
		Notifications: notifs,
		Errors:        errs,
		cancel:        cancel,
	}
}

// runLongpoll drives a single subscription: it requests, interprets each
// response (see parseLongpollResponse), delivers events onto the typed
// channels, and advances since_time. Closes all three channels on exit.
//
// errs has room for the one terminal error, so sending it never blocks.
func runLongpoll(ctx context.Context, src longpollSource, matches chan<- StreamCallbackMatch, notifs chan StreamCallbackNotification, errs chan<- error) {
	defer close(matches)
	defer close(notifs)
	defer close(errs)
	since := src.sinceTime
	idle := 0
	for {
		params := map[string]string{"category": src.category, "timeout": strconv.Itoa(src.timeout)}
		if since > 0 {
			params["since_time"] = strconv.FormatInt(since, 10)
		}
		resp, err := src.fetch(ctx, params)
		if ctx.Err() != nil {
			// Closed or cancelled by the caller: end without an error.
			return
		}
		if err != nil {
			errs <- &AudDConnectionError{Cause: err}
			return
		}
		if resp.HTTPStatus >= httpClientErrorFloor {
			errs <- &AudDAPIError{
				ErrorCode:   0,
				Message:     fmt.Sprintf("Longpoll endpoint returned HTTP %d", resp.HTTPStatus),
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: stringOrJSON(resp),
			}
			return
		}
		page := parseLongpollResponse(resp)
		if page.err != nil {
			errs <- page.err
			return
		}
		for _, ev := range page.events {
			if !deliverLongpollEvent(ctx, ev, matches, notifs) {
				return
			}
		}
		// An events response moves on only through its timestamps: without
		// a newer since_time the next request would return the same events.
		progressed := len(page.events) > 0 && !page.eventsShape
		if page.hasSince && page.since != since {
			since = page.since
			progressed = true
		}
		if progressed {
			idle = 0
			continue
		}
		if !sleepContext(ctx, longpollIdleDelay(idle)) {
			return
		}
		idle++
	}
}

// longpollIdleDelay is the pause after the n-th consecutive response with
// nothing new in it (n counts from 0).
func longpollIdleDelay(n int) time.Duration {
	d := longpollIdleBackoffMin
	for i := 0; i < n && d < longpollIdleBackoffMax; i++ {
		d *= 2
	}
	if d > longpollIdleBackoffMax {
		d = longpollIdleBackoffMax
	}
	return d
}

// sleepContext waits d or until ctx ends; it reports whether the full wait
// passed.
func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// deliverLongpollEvent sends one event to its channel. It reports false when
// ctx ended first.
func deliverLongpollEvent(ctx context.Context, ev longpollEvent, matches chan<- StreamCallbackMatch, notifs chan StreamCallbackNotification) bool {
	switch {
	case ev.match != nil:
		select {
		case matches <- *ev.match:
		case <-ctx.Done():
			return false
		}
	case ev.notification != nil:
		sendNotification(notifs, *ev.notification)
	}
	return ctx.Err() == nil
}

// sendNotification queues n without blocking. When the buffer is full, the
// oldest unread notification is dropped to make room, so a consumer that
// never reads Notifications can't stall the poll.
func sendNotification(notifs chan StreamCallbackNotification, n StreamCallbackNotification) {
	for {
		select {
		case notifs <- n:
			return
		default:
		}
		select {
		case <-notifs:
		default:
		}
	}
}

// longpollEvent is one deliverable event: exactly one field is set.
type longpollEvent struct {
	match        *StreamCallbackMatch
	notification *StreamCallbackNotification
}

// longpollPage is what one longpoll response yields.
type longpollPage struct {
	events   []longpollEvent
	since    int64 // the next since_time, when hasSince
	hasSince bool
	err      error // terminal: stop polling
	// eventsShape is set for an {"events":[…]} response.
	eventsShape bool
}

// parseLongpollResponse interprets one HTTP-200 longpoll body. The server
// answers with one of:
//
//   - {"events":[{"timestamp":<ms>,"category":"…","data":<callback body>}, …]}:
//     each data is parsed like a callback POST body; the newest timestamp is
//     the next since_time. An event whose data doesn't parse, or whose
//     recognition has no songs, is skipped.
//   - {"timeout":"no events before timeout","timestamp":<ms>}: nothing new;
//     the timestamp is the next since_time.
//   - {"error":"…"}: the request itself is invalid; terminal.
//
// A callback body at the top level ({"result":…} or {"notification":…},
// with an optional top-level timestamp) is accepted too and delivered as
// is, including a recognition with an empty `results` list. Anything else
// (empty, non-JSON, an unknown shape) yields an empty page.
func parseLongpollResponse(resp *httpResponse) longpollPage {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(resp.RawBody, &body); err != nil || body == nil {
		return longpollPage{}
	}
	if errRaw, ok := body["error"]; ok && !isJSONNull(errRaw) {
		return longpollPage{err: longpollErrorFromBody(resp, errRaw)}
	}
	if eventsRaw, ok := body["events"]; ok {
		return parseLongpollEvents(eventsRaw)
	}
	_, hasResult := body["result"]
	_, hasNotification := body["notification"]
	if hasResult || hasNotification {
		var page longpollPage
		if ev, ok := parseLongpollEventData(resp.RawBody, true); ok {
			page.events = []longpollEvent{ev}
		}
		page.since, page.hasSince = longpollTimestamp(body["timestamp"])
		return page
	}
	if _, ok := body["timeout"]; ok {
		var page longpollPage
		page.since, page.hasSince = longpollTimestamp(body["timestamp"])
		return page
	}
	return longpollPage{}
}

// parseLongpollEvents handles the `events` list of an event response.
func parseLongpollEvents(raw json.RawMessage) longpollPage {
	page := longpollPage{eventsShape: true}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return page
	}
	for _, item := range items {
		var ev struct {
			Timestamp json.RawMessage `json:"timestamp"`
			Data      json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(item, &ev); err != nil {
			continue
		}
		if ts, ok := longpollTimestamp(ev.Timestamp); ok && (!page.hasSince || ts > page.since) {
			page.since, page.hasSince = ts, true
		}
		if parsed, ok := parseLongpollEventData(ev.Data, false); ok {
			page.events = append(page.events, parsed)
		}
	}
	return page
}

// parseLongpollEventData parses an event's data (a callback body, possibly
// sent as a JSON string). It reports false for data that doesn't parse and,
// unless keepEmpty is set, for a recognition with no songs.
func parseLongpollEventData(data json.RawMessage, keepEmpty bool) (longpollEvent, bool) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || isJSONNull(data) {
		return longpollEvent{}, false
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return longpollEvent{}, false
		}
		data = []byte(s)
	}
	match, notif, songs, err := parseCallback(data)
	switch {
	case err != nil:
		return longpollEvent{}, false
	case match != nil:
		if songs == 0 && !keepEmpty {
			return longpollEvent{}, false
		}
		return longpollEvent{match: match}, true
	case notif != nil:
		return longpollEvent{notification: notif}, true
	}
	return longpollEvent{}, false
}

// longpollErrorFromBody builds the terminal error for an `error` body. A
// text error ({"error":"Invalid timeout arg.  Must be 1-120."}) matches
// ErrInvalidRequest and carries the server's text; an AudD error block
// ({"error":{"error_code":…}}) maps by its code like any API error.
func longpollErrorFromBody(resp *httpResponse, errRaw json.RawMessage) error {
	var block map[string]any
	if err := json.Unmarshal(errRaw, &block); err == nil && resp.JSONBody != nil {
		return raiseFromErrorResponse(resp.JSONBody, resp.HTTPStatus, resp.RequestID, false)
	}
	var msg string
	if err := json.Unmarshal(errRaw, &msg); err != nil {
		msg = string(errRaw)
	}
	if strings.TrimSpace(msg) == "" {
		msg = "Longpoll request rejected"
	}
	return &AudDAPIError{
		ErrorCode:   0,
		Message:     msg,
		HTTPStatus:  resp.HTTPStatus,
		RequestID:   resp.RequestID,
		RawResponse: stringOrJSON(resp),
		sentinel:    ErrInvalidRequest,
	}
}

// longpollTimestamp reads a millisecond timestamp, accepting a JSON number or
// a numeric string. It reports false for anything else and for values <= 0.
func longpollTimestamp(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || isJSONNull(raw) {
		return 0, false
	}
	var s string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false
		}
		s = strings.TrimSpace(s)
	} else {
		s = string(raw)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, n > 0
	}
	if f, ok := strictParseFloat(s); ok && f > 0 && f < 1<<62 {
		return int64(f), true
	}
	return 0, false
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

package audd

import (
	"context"
	"encoding/json"
	"fmt"
)

// Stream-management URL fragments.
const (
	pathSetCallbackURL = "/setCallbackUrl/"
	pathGetCallbackURL = "/getCallbackUrl/"
	pathAddStream      = "/addStream/"
	pathSetStreamURL   = "/setStreamUrl/"
	pathDeleteStream   = "/deleteStream/"
	pathGetStreams     = "/getStreams/"
	pathLongpoll       = "/longpoll/"
)

// noCallbackErrorCode is the AudD error code surfaced when the longpoll-account
// has no callback URL configured. Used by the longpoll preflight.
const noCallbackErrorCode = 19

// preflightNoCallbackHint is the hint returned when the preflight detects the
// "no callback URL" condition. Tells users exactly how to fix it.
const preflightNoCallbackHint = "Longpoll won't deliver events because no callback URL is configured for this account. " +
	"Set one first via client.Streams().SetCallbackUrl(url, opts) — `https://audd.tech/empty/` is fine if you " +
	"only want longpolling and don't need a real receiver. " +
	"To skip this check, set LongpollOptions.SkipCallbackCheck = true."

// StreamsClient handles stream-management methods (set callback URL, add/remove
// streams, list, longpoll). Reach via Client.Streams().
type StreamsClient struct {
	c *Client
}

func newStreamsClient(c *Client) *StreamsClient {
	return &StreamsClient{c: c}
}

// SetCallbackUrlOptions controls SetCallbackUrl.
type SetCallbackUrlOptions struct {
	// ReturnMetadata, if non-empty, is added as a `?return=<csv>` query
	// parameter on the callback URL. If the URL already has a `return`
	// param, the call returns an error rather than silently overwriting.
	ReturnMetadata []string
}

// SetCallbackUrl is the no-context convenience wrapper around
// SetCallbackUrlContext. Defaults to context.Background(). Use
// SetCallbackUrlContext for cancellation, deadlines, or distributed tracing.
func (s *StreamsClient) SetCallbackUrl(urlStr string, opts *SetCallbackUrlOptions) error {
	return s.SetCallbackUrlContext(context.Background(), urlStr, opts)
}

// SetCallbackUrlContext tells AudD to POST recognition results for your
// account to `url`.
func (s *StreamsClient) SetCallbackUrlContext(ctx context.Context, urlStr string, opts *SetCallbackUrlOptions) error {
	finalURL, err := addReturnToURL(urlStr, optsReturnMetadata(opts))
	if err != nil {
		return err
	}
	_, err = s.post(ctx, pathSetCallbackURL, map[string]string{"url": finalURL}, RetryClassMutating)
	return err
}

func optsReturnMetadata(opts *SetCallbackUrlOptions) []string {
	if opts == nil {
		return nil
	}
	return opts.ReturnMetadata
}

// GetCallbackUrl is the no-context convenience wrapper around
// GetCallbackUrlContext. Defaults to context.Background().
func (s *StreamsClient) GetCallbackUrl() (string, error) {
	return s.GetCallbackUrlContext(context.Background())
}

// GetCallbackUrlContext returns the configured callback URL, or "" if none is
// set (server returns error #19 in that case, which we surface as
// ErrInvalidRequest).
func (s *StreamsClient) GetCallbackUrlContext(ctx context.Context) (string, error) {
	result, err := s.post(ctx, pathGetCallbackURL, map[string]string{}, RetryClassRead)
	if err != nil {
		return "", err
	}
	str, _ := result.(string)
	return str, nil
}

// AddStreamRequest describes a stream to subscribe to.
type AddStreamRequest struct {
	// URL is the stream URL. Accepts direct stream URLs (DASH, Icecast,
	// HLS, m3u/m3u8) and shortcuts: twitch:<channel>, youtube:<video_id>,
	// youtube-ch:<channel_id>.
	URL string
	// RadioID is the integer ID you assign to this stream slot.
	RadioID int
	// Callbacks: pass "before" to fire callbacks at song start (default is at song end).
	Callbacks string
}

// Add is the no-context convenience wrapper around AddContext. Defaults to
// context.Background().
func (s *StreamsClient) Add(req AddStreamRequest) error {
	return s.AddContext(context.Background(), req)
}

// AddContext subscribes the account to the given stream.
func (s *StreamsClient) AddContext(ctx context.Context, req AddStreamRequest) error {
	data := map[string]string{"url": req.URL, "radio_id": fmt.Sprint(req.RadioID)}
	if req.Callbacks != "" {
		data["callbacks"] = req.Callbacks
	}
	_, err := s.post(ctx, pathAddStream, data, RetryClassMutating)
	return err
}

// SetURL is the no-context convenience wrapper around SetURLContext. Defaults
// to context.Background().
func (s *StreamsClient) SetURL(radioID int, urlStr string) error {
	return s.SetURLContext(context.Background(), radioID, urlStr)
}

// SetURLContext changes the stream URL for an existing radio_id.
func (s *StreamsClient) SetURLContext(ctx context.Context, radioID int, urlStr string) error {
	_, err := s.post(ctx, pathSetStreamURL, map[string]string{
		"radio_id": fmt.Sprint(radioID), "url": urlStr,
	}, RetryClassMutating)
	return err
}

// Delete is the no-context convenience wrapper around DeleteContext. Defaults
// to context.Background().
func (s *StreamsClient) Delete(radioID int) error {
	return s.DeleteContext(context.Background(), radioID)
}

// DeleteContext removes a stream subscription.
func (s *StreamsClient) DeleteContext(ctx context.Context, radioID int) error {
	_, err := s.post(ctx, pathDeleteStream, map[string]string{
		"radio_id": fmt.Sprint(radioID),
	}, RetryClassMutating)
	return err
}

// List is the no-context convenience wrapper around ListContext. Defaults to
// context.Background().
func (s *StreamsClient) List() ([]Stream, error) {
	return s.ListContext(context.Background())
}

// ListContext returns all configured streams.
func (s *StreamsClient) ListContext(ctx context.Context) ([]Stream, error) {
	result, err := s.post(ctx, pathGetStreams, map[string]string{}, RetryClassRead)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []Stream{}, nil
	}
	raw, mErr := json.Marshal(result)
	if mErr != nil {
		return nil, &AuddSerializationError{Message: mErr.Error()}
	}
	var out []Stream
	if uErr := json.Unmarshal(raw, &out); uErr != nil {
		return nil, &AuddSerializationError{Message: uErr.Error(), RawText: string(raw)}
	}
	return out, nil
}

// DeriveLongpollCategory computes the 9-char longpoll category locally for the
// authenticated client's token + radio_id. No network call. Useful for
// sharing categories with browser/widget code without leaking the api_token.
func (s *StreamsClient) DeriveLongpollCategory(radioID int) string {
	return DeriveLongpollCategory(s.c.apiToken, radioID)
}

// LongpollOptions controls the longpoll iterator. Zero values use sane defaults.
type LongpollOptions struct {
	// SinceTime is the unix timestamp to resume from. 0 means "start from now".
	SinceTime int
	// Timeout is the longpoll timeout in seconds (server-side default: 50).
	Timeout int
	// SkipCallbackCheck disables the preflight that detects the "no callback URL"
	// misconfiguration. Default false (preflight is on).
	SkipCallbackCheck bool
}

// LongpollPoll is an active long-poll subscription. Three typed channels
// surface its output; consume them with `select`. Errors is single-shot and
// closing — when an error fires, the consumer is terminal.
//
// Closing the parent context (or calling Close) tears down the background
// poller and closes all channels.
type LongpollPoll struct {
	// Matches yields one StreamCallbackMatch per recognition. Closed when
	// the poll terminates.
	Matches <-chan StreamCallbackMatch
	// Notifications yields stream-lifecycle events.
	Notifications <-chan StreamCallbackNotification
	// Errors yields a single terminal error and then closes. After an
	// error fires, Matches and Notifications close too.
	Errors <-chan error

	cancel context.CancelFunc
	closed chan struct{}
}

// Close stops the background poll. Idempotent.
func (p *LongpollPoll) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Longpoll is the no-context convenience wrapper around LongpollContext.
// Defaults to context.Background(). Callers wanting cancellation should use
// LongpollContext or rely on the poll's Close() method.
func (s *StreamsClient) Longpoll(category string, opts *LongpollOptions) (*LongpollPoll, error) {
	return s.LongpollContext(context.Background(), category, opts)
}

// LongpollContext starts a long-poll subscription. Returns a LongpollPoll
// whose Matches / Notifications / Errors channels are filled by a background
// goroutine. The goroutine exits when ctx is cancelled, the poll's Close()
// is called, or a terminal error fires (which is sent on Errors then closes
// all three channels).
//
// On entry, runs a preflight GetCallbackUrlContext() unless
// SkipCallbackCheck is set — catches the common silent-failure mode where
// no callback URL is configured.
func (s *StreamsClient) LongpollContext(ctx context.Context, category string, opts *LongpollOptions) (*LongpollPoll, error) {
	if opts == nil {
		opts = &LongpollOptions{}
	}
	if !opts.SkipCallbackCheck {
		if err := s.preflightCallbackURL(ctx); err != nil {
			return nil, err
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 50
	}
	pollCtx, cancel := context.WithCancel(ctx)
	matches := make(chan StreamCallbackMatch)
	notifs := make(chan StreamCallbackNotification)
	errs := make(chan error, 1)
	go runLongpoll(pollCtx, longpollSource{
		fetch: func(ctx context.Context, params map[string]string) (*httpResponse, error) {
			policy := s.c.retryPolicy(RetryClassRead)
			return retryDo(ctx, policy, func() (*httpResponse, error) {
				return s.c.standardHTTP.get(ctx, apiBase+pathLongpoll, params)
			})
		},
		category:  category,
		sinceTime: opts.SinceTime,
		timeout:   timeout,
	}, matches, notifs, errs)
	return &LongpollPoll{
		Matches:       matches,
		Notifications: notifs,
		Errors:        errs,
		cancel:        cancel,
	}, nil
}

// longpollSource abstracts the HTTP fetch so authenticated and tokenless
// consumers share the same dispatch loop.
type longpollSource struct {
	fetch     func(ctx context.Context, params map[string]string) (*httpResponse, error)
	category  string
	sinceTime int
	timeout   int
}

// runLongpoll drives a single subscription. Reads HTTP responses, parses
// each into a Match or Notification, and dispatches onto the typed channels.
// Closes all three channels on exit.
func runLongpoll(ctx context.Context, src longpollSource, matches chan<- StreamCallbackMatch, notifs chan<- StreamCallbackNotification, errs chan<- error) {
	defer close(matches)
	defer close(notifs)
	defer close(errs)
	curSince := src.sinceTime
	for {
		params := map[string]string{"category": src.category, "timeout": fmt.Sprint(src.timeout)}
		if curSince > 0 {
			params["since_time"] = fmt.Sprint(curSince)
		}
		resp, err := src.fetch(ctx, params)
		if err != nil {
			sendErr(ctx, errs, &AuddConnectionError{Cause: err})
			return
		}
		if resp.HTTPStatus >= httpClientErrorFloor {
			sendErr(ctx, errs, &AuddAPIError{
				ErrorCode:   0,
				Message:     fmt.Sprintf("Longpoll endpoint returned HTTP %d", resp.HTTPStatus),
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: stringOrJSON(resp),
			})
			return
		}
		if resp.RawBody == nil {
			sendErr(ctx, errs, &AuddSerializationError{Message: "Longpoll response was empty"})
			return
		}
		// Benign no-events tick: server returns {"timeout":"..."} when nothing
		// happened during the longpoll window. Advance since_time, keep polling.
		if isLongpollKeepalive(resp.JSONBody) {
			if ts, ok := resp.JSONBody["timestamp"].(float64); ok {
				curSince = int(ts)
			}
			continue
		}
		match, notif, parseErr := ParseCallback(resp.RawBody)
		if parseErr != nil {
			sendErr(ctx, errs, parseErr)
			return
		}
		switch {
		case match != nil:
			select {
			case matches <- *match:
			case <-ctx.Done():
				return
			}
		case notif != nil:
			select {
			case notifs <- *notif:
			case <-ctx.Done():
				return
			}
		}
		if resp.JSONBody != nil {
			if ts, ok := resp.JSONBody["timestamp"].(float64); ok {
				curSince = int(ts)
			}
		}
	}
}

func sendErr(ctx context.Context, errs chan<- error, err error) {
	select {
	case errs <- err:
	case <-ctx.Done():
	}
}

// isLongpollKeepalive reports whether body is a `{"timeout":"..."}` no-events
// tick: the server sends one of these every <timeout> seconds when no
// recognition or notification is queued. Consumers shouldn't see these.
func isLongpollKeepalive(body map[string]any) bool {
	if body == nil {
		return false
	}
	if _, hasResult := body["result"]; hasResult {
		return false
	}
	if _, hasNotification := body["notification"]; hasNotification {
		return false
	}
	_, hasTimeout := body["timeout"]
	return hasTimeout
}

// preflightCallbackURL surfaces a friendly error when the account hasn't set
// a callback URL — the silent-failure mode for longpoll subscriptions.
func (s *StreamsClient) preflightCallbackURL(ctx context.Context) error {
	_, err := s.GetCallbackUrlContext(ctx)
	if err == nil {
		return nil
	}
	var apiErr *AuddAPIError
	if asAPIErr(err, &apiErr) && apiErr.ErrorCode == noCallbackErrorCode {
		return &AuddAPIError{
			ErrorCode:  0,
			Message:    preflightNoCallbackHint,
			HTTPStatus: apiErr.HTTPStatus,
			RequestID:  apiErr.RequestID,
		}
	}
	return err
}

// post wraps the common POST + decode-success flow used by all stream methods.
// Returns the body's `result` field (not the whole body) on success.
func (s *StreamsClient) post(ctx context.Context, path string, data map[string]string, class RetryClass) (any, error) {
	policy := s.c.retryPolicy(class)
	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		return s.c.standardHTTP.postForm(ctx, apiBase+path, formFields{Data: data})
	})
	if err != nil {
		return nil, &AuddConnectionError{Cause: err}
	}
	body, err := s.c.decodeOrRaise(resp)
	if err != nil {
		return nil, err
	}
	return body["result"], nil
}

// asAPIErr is a small wrapper to keep call-sites readable.
func asAPIErr(err error, target **AuddAPIError) bool {
	type asAble interface {
		As(any) bool
	}
	_ = asAble(nil)
	return errAs(err, target)
}

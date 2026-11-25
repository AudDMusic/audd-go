package audd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// longpollTimeoutMargin is added on top of the poll timeout when sizing the
// per-request deadline for longpoll GETs.
const longpollTimeoutMargin = 10 * time.Second

// longpollRequestTimeout sizes the HTTP deadline for one longpoll request:
// the server-side poll timeout plus a network margin.
func longpollRequestTimeout(pollTimeoutSeconds int) time.Duration {
	return time.Duration(pollTimeoutSeconds)*time.Second + longpollTimeoutMargin
}

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
	// parameter on the callback URL (e.g. "apple_music,spotify"). If the
	// URL already has a `return` param, the call returns an error rather
	// than silently overwriting.
	ReturnMetadata string
	// ExtraParameters lets you pass additional form fields on the
	// setCallbackUrl call itself (not appended to the callback URL).
	// Typed fields take precedence on collision.
	ExtraParameters map[string]string
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
	data := map[string]string{}
	// ExtraParameters first; typed fields then override on collision.
	if opts != nil {
		for k, v := range opts.ExtraParameters {
			data[k] = v
		}
	}
	data["url"] = finalURL
	_, err = s.post(ctx, "setCallbackUrl", pathSetCallbackURL, data, RetryClassMutating)
	return err
}

func optsReturnMetadata(opts *SetCallbackUrlOptions) string {
	if opts == nil {
		return ""
	}
	return opts.ReturnMetadata
}

// GetCallbackUrl is the no-context convenience wrapper around
// GetCallbackUrlContext. Defaults to context.Background().
func (s *StreamsClient) GetCallbackUrl() (string, error) {
	return s.GetCallbackUrlContext(context.Background())
}

// GetCallbackUrlContext returns the configured callback URL. When no callback
// URL is set for the account, the server responds with error #19 ("Internal
// error" — that response is the no-callback-URL signal, not a real internal
// failure), which surfaces as an *AudDAPIError matching ErrBlocked.
func (s *StreamsClient) GetCallbackUrlContext(ctx context.Context) (string, error) {
	result, err := s.post(ctx, "getCallbackUrl", pathGetCallbackURL, map[string]string{}, RetryClassRead)
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
	// ExtraParameters lets you pass additional form fields the typed
	// options don't cover. Typed fields take precedence on collision.
	ExtraParameters map[string]string
}

// Add is the no-context convenience wrapper around AddContext. Defaults to
// context.Background().
func (s *StreamsClient) Add(req AddStreamRequest) error {
	return s.AddContext(context.Background(), req)
}

// AddContext subscribes the account to the given stream.
func (s *StreamsClient) AddContext(ctx context.Context, req AddStreamRequest) error {
	data := map[string]string{}
	// ExtraParameters first; typed fields then override on collision.
	for k, v := range req.ExtraParameters {
		data[k] = v
	}
	data["url"] = req.URL
	data["radio_id"] = fmt.Sprint(req.RadioID)
	if req.Callbacks != "" {
		data["callbacks"] = req.Callbacks
	}
	_, err := s.post(ctx, "addStream", pathAddStream, data, RetryClassMutating)
	return err
}

// SetURL is the no-context convenience wrapper around SetURLContext. Defaults
// to context.Background().
func (s *StreamsClient) SetURL(radioID int, urlStr string) error {
	return s.SetURLContext(context.Background(), radioID, urlStr)
}

// SetURLContext changes the stream URL for an existing radio_id.
func (s *StreamsClient) SetURLContext(ctx context.Context, radioID int, urlStr string) error {
	_, err := s.post(ctx, "setStreamUrl", pathSetStreamURL, map[string]string{
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
	_, err := s.post(ctx, "deleteStream", pathDeleteStream, map[string]string{
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
	result, err := s.post(ctx, "getStreams", pathGetStreams, map[string]string{}, RetryClassRead)
	if err != nil {
		return nil, err
	}
	// A result that isn't a list means no streams; an entry that doesn't
	// decode is skipped.
	return decodeLenientList[Stream](result), nil
}

// DeriveLongpollCategory computes the 9-char longpoll category locally for the
// authenticated client's token + radio_id. No network call. Useful for
// sharing categories with browser/widget code without leaking the api_token.
func (s *StreamsClient) DeriveLongpollCategory(radioID int) string {
	return DeriveLongpollCategory(s.c.APIToken(), radioID)
}

// LongpollOptions controls the longpoll iterator. Zero values use sane defaults.
type LongpollOptions struct {
	// SinceTime is a server timestamp in milliseconds since the Unix epoch,
	// as carried by an earlier longpoll response: only events newer than it
	// are delivered. 0 means "start from now".
	SinceTime int
	// Timeout is how long, in seconds, the server holds each longpoll request
	// open while waiting for events: 1 to 120. 0 means 50.
	Timeout int
	// SkipCallbackCheck disables the preflight that detects the "no callback URL"
	// misconfiguration. Default false (preflight is on).
	SkipCallbackCheck bool
}

// LongpollPoll is an active long-poll subscription. Three typed channels
// surface its output; consume them with `select`. Errors closes when the poll
// ends, after sending the terminal error if there is one, so it is the
// natural exit for the loop.
//
// Each longpoll response that carries events delivers them in order, and the
// next request asks only for events newer than the newest one. A response
// with nothing new in it (neither an event nor a new server timestamp) is
// followed by a pause that grows from 1 second to 30 seconds, so a
// misbehaving response never turns into a busy loop.
//
// The poll ends when its context is cancelled, Close is called, or a
// terminal error is sent on Errors. Cancelling or closing aborts a request
// that is waiting and closes all three channels without sending an error.
type LongpollPoll struct {
	// Matches yields one StreamCallbackMatch per recognition. Closed when
	// the poll ends.
	Matches <-chan StreamCallbackMatch
	// Notifications yields stream-lifecycle events. It holds up to 16 unread
	// notifications; when it is full, the oldest unread one is dropped, so a
	// loop that reads only Matches never stalls the poll. Closed when the
	// poll ends.
	Notifications <-chan StreamCallbackNotification
	// Errors yields at most one terminal error and then closes; Matches and
	// Notifications close with it. The terminal errors are:
	//
	//   - *AudDAPIError matching ErrInvalidRequest: the server rejected the
	//     request itself (for example a Timeout outside 1-120); Message
	//     carries the server's text. Sending the same request again would
	//     get the same answer, so the poll stops. An AudD error block
	//     ({"status":"error","error":{...}}) maps by its code instead, like
	//     any API error.
	//   - *AudDAPIError matching ErrServer: an HTTP error status (408, 429
	//     and 5xx are retried first, within the retry budget).
	//   - *AudDConnectionError: a network failure that persisted through the
	//     retries.
	Errors <-chan error

	cancel context.CancelFunc
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

// LongpollByRadioID is the no-context convenience wrapper around
// LongpollByRadioIDContext. Defaults to context.Background().
//
// One-step entry point for the common case where the caller has the
// api_token + radio_id and wants a longpoll subscription without computing
// the category by hand. Equivalent to:
//
//	category := audd.DeriveLongpollCategory(token, radioID)
//	poll, err := client.Streams().Longpoll(category, opts)
//
// The category-string form (Longpoll / LongpollContext) is still the path
// for tokenless consumers — the server derives the category and ships only
// the 9-char string to a browser/mobile/embedded client.
func (s *StreamsClient) LongpollByRadioID(radioID int, opts *LongpollOptions) (*LongpollPoll, error) {
	return s.LongpollByRadioIDContext(context.Background(), radioID, opts)
}

// LongpollByRadioIDContext derives the longpoll category locally from the
// authenticated client's token + radioID and starts a long-poll subscription.
// Same retry/timeout/preflight policy as LongpollContext.
func (s *StreamsClient) LongpollByRadioIDContext(ctx context.Context, radioID int, opts *LongpollOptions) (*LongpollPoll, error) {
	return s.LongpollContext(ctx, s.DeriveLongpollCategory(radioID), opts)
}

// LongpollContext starts a long-poll subscription. Returns a LongpollPoll
// whose Matches / Notifications / Errors channels are filled by a background
// goroutine. The goroutine exits when ctx is cancelled, the poll's Close()
// is called, or a terminal error fires (which is sent on Errors then closes
// all three channels). See LongpollPoll for how responses are handled.
//
// The longpoll requests carry only the category, never the api_token: the
// category alone authorizes them.
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
		timeout = defaultLongpollTimeout
	}
	return startLongpoll(ctx, longpollSource{
		fetch: func(ctx context.Context, params map[string]string) (*httpResponse, error) {
			policy := s.c.retryPolicy(RetryClassRead)
			return retryDo(ctx, policy, func() (*httpResponse, error) {
				// Longpoll requests are sized to the poll timeout plus a
				// margin — a poll timeout above the standard 60s HTTP
				// timeout must not be cut short by the transport.
				reqCtx, cancel := context.WithTimeout(ctx, longpollRequestTimeout(timeout))
				defer cancel()
				return s.c.longpollHTTP.getNoToken(reqCtx, apiBase+pathLongpoll, params)
			})
		},
		category:  category,
		sinceTime: int64(opts.SinceTime),
		timeout:   timeout,
	}), nil
}

// preflightCallbackURL surfaces a friendly error when the account hasn't set
// a callback URL — the silent-failure mode for longpoll subscriptions.
//
// The no-callback-URL signal is error #19 with the "Internal error" message.
// Code 19 also covers real conditions (maintenance, blocked requests), so the
// rewrite only happens when the message indicates the no-callback case; any
// other #19 passes through unchanged.
func (s *StreamsClient) preflightCallbackURL(ctx context.Context) error {
	_, err := s.GetCallbackUrlContext(ctx)
	if err == nil {
		return nil
	}
	var apiErr *AudDAPIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode == noCallbackErrorCode && indicatesNoCallbackURL(apiErr.Message) {
		return &AudDAPIError{
			ErrorCode:  0,
			Message:    preflightNoCallbackHint,
			HTTPStatus: apiErr.HTTPStatus,
			RequestID:  apiErr.RequestID,
		}
	}
	return err
}

// indicatesNoCallbackURL reports whether a code-19 error message from
// getCallbackUrl is the no-callback-URL signal ("Internal error") rather than
// a real server condition (maintenance, blocked request, abuse, ...).
func indicatesNoCallbackURL(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "internal") || strings.Contains(lower, "callback")
}

// post wraps the common POST + decode-success flow used by all stream methods.
// Returns the body's `result` field (not the whole body) on success.
func (s *StreamsClient) post(ctx context.Context, method, path string, data map[string]string, class RetryClass) (any, error) {
	policy := s.c.retryPolicy(class)
	endpoint := apiBase + path
	obs := s.c.observeCall(method, endpoint)
	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		return s.c.standardHTTP.postForm(ctx, endpoint, formFields{Data: data})
	})
	if err != nil {
		obs.fail()
		return nil, &AudDConnectionError{Cause: err}
	}
	obs.done(resp)
	body, err := s.c.decodeOrRaise(resp)
	if err != nil {
		return nil, err
	}
	return body["result"], nil
}

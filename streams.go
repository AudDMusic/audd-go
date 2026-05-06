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

// ParseCallback parses a callback POST body into a typed payload. Use in your
// HTTP handler that receives AudD callbacks. The handler/framework remains
// your choice — we provide the parser, not the framework wrapper.
func (s *StreamsClient) ParseCallback(body []byte) (*StreamCallbackPayload, error) {
	return ParseCallback(body)
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

// LongpollEvent is one event in the longpoll iterator stream. On error, Err
// is non-nil and the channel is then closed.
type LongpollEvent struct {
	// Body is the raw decoded JSON object.
	Body map[string]any
	// Err is set when the event represents a terminal failure.
	Err error
}

// Longpoll is the no-context convenience wrapper around LongpollContext.
// Defaults to context.Background(). Callers wanting cancellation or a
// deadline (e.g. to stop the background goroutine without draining the
// channel) should use LongpollContext.
func (s *StreamsClient) Longpoll(category string, opts *LongpollOptions) (<-chan LongpollEvent, error) {
	return s.LongpollContext(context.Background(), category, opts)
}

// LongpollContext returns a channel that yields raw JSON longpoll events. The
// channel is closed when ctx is cancelled or a terminal error occurs.
//
// On entry, runs a preflight `GetCallbackUrlContext()` unless
// `SkipCallbackCheck` is set; this catches the most common silent-failure
// mode where no callback URL is configured.
func (s *StreamsClient) LongpollContext(ctx context.Context, category string, opts *LongpollOptions) (<-chan LongpollEvent, error) {
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
	out := make(chan LongpollEvent)
	go func() {
		defer close(out)
		curSince := opts.SinceTime
		for {
			params := map[string]string{"category": category, "timeout": fmt.Sprint(timeout)}
			if curSince > 0 {
				params["since_time"] = fmt.Sprint(curSince)
			}
			policy := s.c.retryPolicy(RetryClassRead)
			resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
				return s.c.standardHTTP.get(ctx, apiBase+pathLongpoll, params)
			})
			if err != nil {
				select {
				case out <- LongpollEvent{Err: &AuddConnectionError{Cause: err}}:
				case <-ctx.Done():
				}
				return
			}
			if resp.JSONBody == nil {
				select {
				case out <- LongpollEvent{Err: &AuddSerializationError{Message: "Unparseable longpoll response", RawText: string(resp.RawBody)}}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case out <- LongpollEvent{Body: resp.JSONBody}:
			case <-ctx.Done():
				return
			}
			if ts, ok := resp.JSONBody["timestamp"].(float64); ok {
				curSince = int(ts)
			}
		}
	}()
	return out, nil
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

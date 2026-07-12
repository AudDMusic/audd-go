package audd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// API base URLs.
const (
	apiBase        = "https://api.audd.io"
	enterpriseBase = "https://enterprise.audd.io"
)

// Environment variable consulted when NewClient is called with an empty token.
const tokenEnvVar = "AUDD_API_TOKEN"

// Code 51 is server-side soft-deprecation: a deprecated parameter was used,
// but the request was fulfilled. We log a warning and pass the result
// through.
const deprecatedParamsCode = 51

// httpClientErrorFloor is the HTTP status floor we treat as "definitely an error".
const httpClientErrorFloor = 400

// AudDEvent is emitted by the SDK request lifecycle when an OnEvent hook is
// registered. Frozen plain-data; never carries the api_token or request body
// bytes.
type AudDEvent struct {
	Kind       string // "request" | "response" | "exception"
	Method     string // AudD method name, e.g. "recognize", "addStream"
	URL        string
	RequestID  string // X-Request-Id header value, if present
	HTTPStatus int    // 0 for request/exception kinds
	Elapsed    time.Duration
	ErrorCode  int // AudD error_code, if any
	Extras     map[string]any
}

// OnEventHook is the signature for the inspection hook. Hook errors are
// swallowed by the SDK so observability never breaks the request path.
type OnEventHook func(AudDEvent)

// Option configures a Client. Pass via NewClient(token, opts...).
type Option func(*Client)

// WithHTTPClient injects a caller-managed *http.Client. Useful for corporate
// proxies, mTLS, observability. The same client is used for both the
// standard and enterprise transports.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.userHTTPClient = hc }
}

// WithMaxAttempts overrides the default retry attempt count for all classes.
func WithMaxAttempts(n int) Option {
	return func(c *Client) { c.maxAttempts = n }
}

// WithBackoffFactor overrides the default initial-backoff for retries.
func WithBackoffFactor(d time.Duration) Option {
	return func(c *Client) { c.backoffFactor = d }
}

// WithStandardTimeout overrides the per-request timeout for non-enterprise endpoints.
func WithStandardTimeout(d time.Duration) Option {
	return func(c *Client) { c.standardTimeout = d }
}

// WithEnterpriseTimeout overrides the per-request timeout for enterprise endpoints.
func WithEnterpriseTimeout(d time.Duration) Option {
	return func(c *Client) { c.enterpriseTimeout = d }
}

// WithOnDeprecation registers a hook called when the server returns a code-51
// deprecation warning alongside a usable result. Default is a stdlib log.Println.
// Pass a no-op to silence.
func WithOnDeprecation(fn func(string)) Option {
	return func(c *Client) { c.onDeprecation = fn }
}

// WithOnEvent registers an inspection hook that receives lifecycle events
// (request/response/exception) for every API call. Off by default.
// Hook panics/errors are recovered and ignored.
func WithOnEvent(fn OnEventHook) Option {
	return func(c *Client) { c.onEvent = fn }
}

// Client is the top-level AudD SDK client. Construct with NewClient(token, ...).
//
// Methods that perform I/O take a context.Context as their first argument and
// honor its deadline / cancellation. Sub-clients (Streams, CustomCatalog,
// Advanced) are accessed via methods on this Client.
//
// Client is safe for concurrent use; sub-clients share state.
type Client struct {
	tokenMu  sync.RWMutex
	apiToken string

	standardTimeout   time.Duration
	enterpriseTimeout time.Duration
	maxAttempts       int
	backoffFactor     time.Duration
	userHTTPClient    *http.Client
	onDeprecation     func(string)
	onEvent           OnEventHook

	standardHTTP   *httpClient
	enterpriseHTTP *httpClient
	// longpollHTTP has no client-level timeout; each longpoll request gets a
	// per-request deadline sized to the poll timeout plus a margin, so poll
	// timeouts above the standard 60s work.
	longpollHTTP *httpClient

	streams        *StreamsClient
	customCatalog  *CustomCatalogClient
	advancedClient *AdvancedClient
}

// NewClient builds an AudD client. Use the public token "test" for examples
// (capped at 10 requests). For production, get a token from
// dashboard.audd.io.
//
// If token is empty, the SDK falls back to the AUDD_API_TOKEN environment
// variable. If that's also empty, NewClient returns a Client with an empty
// token; the first API call will fail with a clear error. Use NewClientStrict
// if you want construction-time validation.
func NewClient(token string, opts ...Option) *Client {
	if token == "" {
		token = os.Getenv(tokenEnvVar)
	}
	c := &Client{
		apiToken:          token,
		standardTimeout:   defaultStandardTimeout,
		enterpriseTimeout: defaultEnterpriseTimeout,
		maxAttempts:       3,
		backoffFactor:     500 * time.Millisecond,
		onDeprecation: func(msg string) {
			log.Printf("audd: deprecation: %s", msg)
		},
	}
	for _, o := range opts {
		o(c)
	}
	c.standardHTTP = newHTTPClient(token, c.standardTimeout, c.userHTTPClient)
	c.enterpriseHTTP = newHTTPClient(token, c.enterpriseTimeout, c.userHTTPClient)
	c.longpollHTTP = newHTTPClient(token, 0, c.userHTTPClient)
	// Sub-clients are initialized eagerly so concurrent first accesses are safe.
	c.streams = newStreamsClient(c)
	c.customCatalog = newCustomCatalogClient(c)
	c.advancedClient = newAdvancedClient(c)
	return c
}

// ErrMissingAPIToken is returned by NewClientStrict when neither a token
// argument nor the AUDD_API_TOKEN env var is set.
var ErrMissingAPIToken = errors.New(
	"audd: api_token not supplied and AUDD_API_TOKEN env var is unset; " +
		"get a token at https://dashboard.audd.io",
)

// NewClientStrict is NewClient with construction-time validation: returns
// ErrMissingAPIToken when no token is available.
func NewClientStrict(token string, opts ...Option) (*Client, error) {
	if token == "" {
		token = os.Getenv(tokenEnvVar)
	}
	if token == "" {
		return nil, ErrMissingAPIToken
	}
	return NewClient(token, opts...), nil
}

// APIToken returns the in-effect token (after any rotation).
func (c *Client) APIToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.apiToken
}

// SetAPIToken atomically swaps the token used for subsequent requests.
// In-flight requests continue with the old token.
func (c *Client) SetAPIToken(newToken string) error {
	if newToken == "" {
		return errors.New("audd: SetAPIToken requires a non-empty token")
	}
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.apiToken = newToken
	c.standardHTTP.setAPIToken(newToken)
	c.enterpriseHTTP.setAPIToken(newToken)
	c.longpollHTTP.setAPIToken(newToken)
	return nil
}

// emitEvent invokes the OnEvent hook (if set) with panic recovery — the hook
// must never break the request path.
func (c *Client) emitEvent(e AudDEvent) {
	if c.onEvent == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	c.onEvent(e)
}

// callObserver ties the request/response/exception lifecycle events of one
// API call together. Create with observeCall; finish with done or fail.
type callObserver struct {
	c           *Client
	method, url string
	start       time.Time
}

// observeCall emits the "request" event and returns an observer for the
// matching completion event.
func (c *Client) observeCall(method, url string) *callObserver {
	c.emitEvent(AudDEvent{Kind: "request", Method: method, URL: url})
	return &callObserver{c: c, method: method, url: url, start: time.Now()}
}

// fail emits the "exception" completion event.
func (o *callObserver) fail() {
	o.c.emitEvent(AudDEvent{
		Kind: "exception", Method: o.method, URL: o.url,
		Elapsed: time.Since(o.start),
	})
}

// done emits the "response" completion event.
func (o *callObserver) done(resp *httpResponse) {
	o.c.emitEvent(AudDEvent{
		Kind: "response", Method: o.method, URL: o.url,
		RequestID: resp.RequestID, HTTPStatus: resp.HTTPStatus,
		Elapsed: time.Since(o.start),
	})
}

// applyCallTimeout applies a per-call deadline when timeout > 0. The returned
// cancel func must always be called.
func applyCallTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}

// Close releases connections owned by the client. Safe to call multiple times.
// Implements io.Closer.
func (c *Client) Close() error {
	if c.standardHTTP != nil {
		_ = c.standardHTTP.Close()
	}
	if c.enterpriseHTTP != nil {
		_ = c.enterpriseHTTP.Close()
	}
	if c.longpollHTTP != nil {
		_ = c.longpollHTTP.Close()
	}
	return nil
}

// retryPolicy returns a fresh policy of the given class using the client's overrides.
func (c *Client) retryPolicy(class RetryClass) RetryPolicy {
	return RetryPolicy{
		Class:         class,
		MaxAttempts:   c.maxAttempts,
		BackoffFactor: c.backoffFactor,
	}
}

// Streams returns the streams sub-client.
func (c *Client) Streams() *StreamsClient {
	return c.streams
}

// CustomCatalog returns the custom-catalog sub-client.
func (c *Client) CustomCatalog() *CustomCatalogClient {
	return c.customCatalog
}

// Advanced returns the advanced sub-client (lyrics + escape-hatch raw_request).
// Uses the RECOGNITION retry policy because find_lyrics is metered.
func (c *Client) Advanced() *AdvancedClient {
	return c.advancedClient
}

// RecognizeOptions controls the standard recognize endpoint.
type RecognizeOptions struct {
	// ReturnMetadata is the comma-separated list of metadata sources to include
	// (e.g. "apple_music,spotify"). Valid values: "apple_music", "spotify",
	// "deezer", "napster", "musicbrainz".
	ReturnMetadata string
	// Market is the ISO country code (server default: "us").
	Market string
	// Timeout bounds this call: the request is cancelled once the duration
	// elapses. A client-wide timeout (WithStandardTimeout) still applies if
	// it is shorter. Zero means no per-call bound.
	Timeout time.Duration
	// ExtraParameters lets you pass additional form fields the typed
	// options don't cover — undocumented parameters, beta features, or
	// any other API field not yet surfaced as a typed property. Typed
	// fields take precedence: if a key here collides with a typed field
	// that's been set, the typed value wins.
	ExtraParameters map[string]string
}

// EnterpriseOptions controls the enterprise recognize endpoint.
type EnterpriseOptions struct {
	// ReturnMetadata is the comma-separated list of metadata sources to include
	// (e.g. "apple_music,spotify"). Valid values: "apple_music", "spotify",
	// "deezer", "napster", "musicbrainz".
	ReturnMetadata   string
	Skip             *int
	Every            *int
	Limit            *int
	SkipFirstSeconds *int
	UseTimecode      *bool
	// AccurateOffsets requests precise within-fragment offsets. It is on by
	// default: leave it nil and the request sends accurate_offsets=true. Set
	// it to a non-nil false to opt out.
	AccurateOffsets *bool
	// Timeout bounds this call: the request is cancelled once the duration
	// elapses. A client-wide timeout (WithEnterpriseTimeout) still applies
	// if it is shorter. Zero means no per-call bound.
	Timeout time.Duration
	// ExtraParameters lets you pass additional form fields the typed
	// options don't cover. Typed fields take precedence on collision.
	ExtraParameters map[string]string
}

// Recognize is the no-context convenience wrapper around RecognizeContext.
// Defaults to context.Background(). Use RecognizeContext for cancellation,
// deadlines, or distributed tracing.
//
// Source can be a URL string, a file path string, []byte, or io.Reader.
// See the Source type for details.
func (c *Client) Recognize(source Source, opts *RecognizeOptions) (*Recognition, error) {
	return c.RecognizeContext(context.Background(), source, opts)
}

// RecognizeContext sends the source to the standard recognize endpoint and
// returns the typed result. Returns (nil, nil) when the server returns
// status=success with result=null (no match).
//
// Source can be a URL string, a file path string, []byte, or io.Reader.
// See the Source type for details.
func (c *Client) RecognizeContext(ctx context.Context, source Source, opts *RecognizeOptions) (*Recognition, error) {
	reopen, err := prepareSource(source)
	if err != nil {
		return nil, err
	}
	if opts != nil {
		var cancel context.CancelFunc
		ctx, cancel = applyCallTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	policy := c.retryPolicy(RetryClassRecognition)
	endpoint := apiBase + "/"
	obs := c.observeCall("recognize", endpoint)

	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		fields, err := reopen()
		if err != nil {
			return nil, err
		}
		c.applyRecognizeOpts(&fields, opts)
		return c.standardHTTP.postForm(ctx, endpoint, fields)
	})
	if err != nil {
		obs.fail()
		return nil, &AudDConnectionError{Cause: err}
	}
	obs.done(resp)
	body, err := c.decodeOrRaise(resp)
	if err != nil {
		return nil, err
	}
	resultRaw, ok := body["result"]
	if !ok || resultRaw == nil {
		return nil, nil
	}
	resultBytes, mErr := json.Marshal(resultRaw)
	if mErr != nil {
		return nil, &AudDSerializationError{Message: "marshal result: " + mErr.Error()}
	}
	var rec Recognition
	if uErr := json.Unmarshal(resultBytes, &rec); uErr != nil {
		return nil, &AudDSerializationError{Message: "decode result: " + uErr.Error(), RawText: string(resultBytes)}
	}
	return &rec, nil
}

// applyRecognizeOpts mutates fields with the standard recognize options.
func (c *Client) applyRecognizeOpts(fields *formFields, opts *RecognizeOptions) {
	if opts == nil {
		return
	}
	if fields.Data == nil {
		fields.Data = map[string]string{}
	}
	// ExtraParameters first; typed fields then override on collision.
	for k, v := range opts.ExtraParameters {
		fields.Data[k] = v
	}
	if opts.ReturnMetadata != "" {
		fields.Data["return"] = opts.ReturnMetadata
	}
	if opts.Market != "" {
		fields.Data["market"] = opts.Market
	}
}

// RecognizeEnterprise sends the source to the enterprise endpoint and returns
// the flattened list of matches. Empty slice when no matches were found.
// RecognizeEnterprise is the no-context convenience wrapper. Defaults to
// context.Background(). Use RecognizeEnterpriseContext for cancellation /
// deadline support — recommended for the enterprise endpoint, which can take
// up to an hour for very large files.
func (c *Client) RecognizeEnterprise(source Source, opts *EnterpriseOptions) ([]EnterpriseMatch, error) {
	return c.RecognizeEnterpriseContext(context.Background(), source, opts)
}

// RecognizeEnterpriseContext sends the source to the enterprise recognize
// endpoint and returns the matches across all chunks of the response.
func (c *Client) RecognizeEnterpriseContext(ctx context.Context, source Source, opts *EnterpriseOptions) ([]EnterpriseMatch, error) {
	reopen, err := prepareSource(source)
	if err != nil {
		return nil, err
	}
	if opts != nil {
		var cancel context.CancelFunc
		ctx, cancel = applyCallTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	policy := c.retryPolicy(RetryClassRecognition)
	endpoint := enterpriseBase + "/"
	obs := c.observeCall("recognizeEnterprise", endpoint)

	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		fields, err := reopen()
		if err != nil {
			return nil, err
		}
		c.applyEnterpriseOpts(&fields, opts)
		return c.enterpriseHTTP.postForm(ctx, endpoint, fields)
	})
	if err != nil {
		obs.fail()
		return nil, &AudDConnectionError{Cause: err}
	}
	obs.done(resp)
	body, err := c.decodeOrRaise(resp)
	if err != nil {
		return nil, err
	}
	chunksRaw, ok := body["result"]
	if !ok || chunksRaw == nil {
		return []EnterpriseMatch{}, nil
	}
	chunksBytes, mErr := json.Marshal(chunksRaw)
	if mErr != nil {
		return nil, &AudDSerializationError{Message: "marshal enterprise result: " + mErr.Error()}
	}
	var chunks []EnterpriseChunkResult
	if uErr := json.Unmarshal(chunksBytes, &chunks); uErr != nil {
		return nil, &AudDSerializationError{Message: "decode enterprise result: " + uErr.Error()}
	}
	out := []EnterpriseMatch{}
	for _, ch := range chunks {
		base, ok := parseOffsetToSeconds(ch.Offset)
		if ok {
			for i := range ch.Songs {
				start := base + float64(ch.Songs[i].StartOffset)/1000
				end := base + float64(ch.Songs[i].EndOffset)/1000
				ch.Songs[i].StartSeconds = &start
				ch.Songs[i].EndSeconds = &end
			}
		}
		out = append(out, ch.Songs...)
	}
	return out, nil
}

// applyEnterpriseOpts mutates fields with the enterprise options.
//
// Accurate offsets default on: the wire carries accurate_offsets=true unless
// the caller explicitly set AccurateOffsets to a non-nil false. A zero-value
// options struct (or nil opts) therefore requests accurate offsets.
func (c *Client) applyEnterpriseOpts(fields *formFields, opts *EnterpriseOptions) {
	if fields.Data == nil {
		fields.Data = map[string]string{}
	}
	if opts == nil {
		fields.Data["accurate_offsets"] = "true"
		return
	}
	// ExtraParameters first; typed fields then override on collision.
	for k, v := range opts.ExtraParameters {
		fields.Data[k] = v
	}
	if opts.ReturnMetadata != "" {
		fields.Data["return"] = opts.ReturnMetadata
	}
	if opts.Skip != nil {
		fields.Data["skip"] = strconv.Itoa(*opts.Skip)
	}
	if opts.Every != nil {
		fields.Data["every"] = strconv.Itoa(*opts.Every)
	}
	if opts.Limit != nil {
		fields.Data["limit"] = strconv.Itoa(*opts.Limit)
	}
	if opts.SkipFirstSeconds != nil {
		fields.Data["skip_first_seconds"] = strconv.Itoa(*opts.SkipFirstSeconds)
	}
	if opts.UseTimecode != nil {
		fields.Data["use_timecode"] = boolStr(*opts.UseTimecode)
	}
	// Default accurate offsets on; only a non-nil explicit false disables.
	if opts.AccurateOffsets != nil {
		fields.Data["accurate_offsets"] = boolStr(*opts.AccurateOffsets)
	} else {
		fields.Data["accurate_offsets"] = "true"
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// decodeOrRaise inspects an HTTP response and:
//
//   - For non-2xx with a non-JSON body: returns *AudDAPIError mapped to ErrServer.
//   - For 2xx with a non-JSON body: returns *AudDSerializationError.
//   - For status=error with code 51 + a usable result: emits a deprecation
//     log via OnDeprecation and falls through.
//   - For status=error otherwise: returns the typed error.
//   - For status=success: returns the body.
func (c *Client) decodeOrRaise(resp *httpResponse) (map[string]any, error) {
	if resp.JSONBody == nil {
		if resp.HTTPStatus >= httpClientErrorFloor {
			return nil, &AudDAPIError{
				ErrorCode:   0,
				Message:     fmt.Sprintf("HTTP %d with non-JSON response body", resp.HTTPStatus),
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: string(resp.RawBody),
			}
		}
		return nil, &AudDSerializationError{Message: "Unparseable response", RawText: string(resp.RawBody)}
	}
	body := resp.JSONBody
	c.maybeWarnAndStrip(body)
	switch body["status"] {
	case "error":
		return nil, raiseFromErrorResponse(body, resp.HTTPStatus, resp.RequestID, false)
	case "success":
		return body, nil
	default:
		return nil, &AudDAPIError{
			ErrorCode:   0,
			Message:     fmt.Sprintf("Unexpected response status: %v", body["status"]),
			HTTPStatus:  resp.HTTPStatus,
			RequestID:   resp.RequestID,
			RawResponse: body,
		}
	}
}

// maybeWarnAndStrip implements the code-51 deprecation pass-through. If the
// body has a usable `result` field, we emit a deprecation message and rewrite
// the body to look like a success response so downstream parsers can run.
func (c *Client) maybeWarnAndStrip(body map[string]any) {
	errBlock, _ := body["error"].(map[string]any)
	codeF, _ := errBlock["error_code"].(float64)
	if int(codeF) != deprecatedParamsCode {
		return
	}
	if _, hasResult := body["result"]; !hasResult || body["result"] == nil {
		return
	}
	msg, _ := errBlock["error_message"].(string)
	if c.onDeprecation != nil {
		c.onDeprecation(msg)
	}
	delete(body, "error")
	body["status"] = "success"
}

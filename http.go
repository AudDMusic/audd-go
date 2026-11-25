package audd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"time"
)

// Default timeouts. Standard endpoints use 60s; the enterprise endpoint can
// take up to an hour for very large files.
const (
	defaultStandardTimeout   = 60 * time.Second
	defaultEnterpriseTimeout = 60 * time.Minute
	defaultConnectTimeout    = 30 * time.Second
)

// userAgent returns the SDK identification header value.
func userAgent() string {
	return fmt.Sprintf("audd-go/%s go/%s (%s)", Version, runtime.Version(), runtime.GOOS)
}

// httpResponse is the SDK-internal wrapper around a parsed AudD HTTP response.
type httpResponse struct {
	JSONBody   map[string]any
	HTTPStatus int
	RequestID  string
	RawBody    []byte
}

// fileField describes a file part in a multipart upload.
type fileField struct {
	Name        string
	ContentType string
	Reader      io.Reader
}

// formFields is the per-attempt request payload built by a source re-opener.
// `Data` are simple form fields. `File` is optional.
type formFields struct {
	Data map[string]string
	File *fileField
}

// httpClient performs the multipart form POSTs and the longpoll GETs against
// AudD's HTTP API. POSTs carry the api_token as a form field; GETs never
// carry it.
type httpClient struct {
	tokenMu  sync.RWMutex
	apiToken string
	hc       *http.Client
	owned    bool // true when we created hc ourselves and should Close it
	// timeout bounds each request: through hc.Timeout on an owned client,
	// and through a per-request context deadline on an injected one. 0 means
	// no SDK-side bound.
	timeout time.Duration
}

// setAPIToken atomically swaps the api_token used for subsequent requests.
// Called via Client.SetAPIToken.
func (h *httpClient) setAPIToken(newToken string) {
	h.tokenMu.Lock()
	h.apiToken = newToken
	h.tokenMu.Unlock()
}

// currentAPIToken reads the in-effect api_token under the RWMutex.
func (h *httpClient) currentAPIToken() string {
	h.tokenMu.RLock()
	defer h.tokenMu.RUnlock()
	return h.apiToken
}

// newHTTPClient builds a transport with the given timeout. Callers may pass
// their own *http.Client (corporate proxy, mTLS, observability sidecar,
// etc.) and we honor it; otherwise we build a fresh one with sensible
// defaults. Either way, each request is bounded by timeout (when > 0).
func newHTTPClient(apiToken string, timeout time.Duration, hc *http.Client) *httpClient {
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
		return &httpClient{apiToken: apiToken, hc: hc, owned: true, timeout: timeout}
	}
	return &httpClient{apiToken: apiToken, hc: hc, owned: false, timeout: timeout}
}

// requestContext applies the per-request deadline for an injected client
// (an owned client already carries it as hc.Timeout). The returned cancel
// func must always be called.
func (h *httpClient) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if !h.owned && h.timeout > 0 {
		return context.WithTimeout(ctx, h.timeout)
	}
	return ctx, func() {}
}

// postForm POSTs `fields` as multipart/form-data with `api_token` always added.
// Builds the request body in a buffer (fully synchronous; no streaming
// upload — AudD's audio uploads are bounded and we need a deterministic
// Content-Length).
func (h *httpClient) postForm(ctx context.Context, target string, fields formFields) (*httpResponse, error) {
	// The file reader is consumed by this attempt; when it owns a resource
	// (io.Closer — e.g. a file the SDK opened from a path source), release it
	// once the attempt finishes. Caller-supplied readers are wrapped so their
	// resources are never closed on the caller's behalf (see prepareReaderSource).
	if fields.File != nil {
		if closer, ok := fields.File.Reader.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("api_token", h.currentAPIToken()); err != nil {
		return nil, err
	}
	for k, v := range fields.Data {
		if err := mw.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if fields.File != nil {
		fw, err := mw.CreateFormFile("file", fields.File.Name)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(fw, fields.File.Reader); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	ctx, cancel := h.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", userAgent())
	return h.do(req)
}

// getNoToken sends a GET with query params and without the api_token. The
// longpoll endpoint (the only GET) is authorized by its category alone.
func (h *httpClient) getNoToken(ctx context.Context, target string, params map[string]string) (*httpResponse, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	ctx, cancel := h.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent())
	return h.do(req)
}

func (h *httpClient) do(req *http.Request) (*httpResponse, error) {
	res, err := h.hc.Do(req)
	if err != nil {
		return nil, redactURLError(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	out := &httpResponse{
		HTTPStatus: res.StatusCode,
		RequestID:  requestIDFromHeaders(res.Header),
		RawBody:    body,
	}
	if len(body) > 0 {
		var parsed map[string]any
		if jsonErr := json.Unmarshal(body, &parsed); jsonErr == nil {
			out.JSONBody = parsed
		}
	}
	return out, nil
}

// redactURLError drops the query string from the URL that net/http puts in
// its transport errors, so an error message or log line never shows query
// parameters. The *url.Error type and its cause are kept, so errors.As and
// errors.Is keep working.
func redactURLError(err error) error {
	ue, ok := err.(*url.Error)
	if !ok {
		return err
	}
	u, perr := url.Parse(ue.URL)
	if perr != nil {
		return &url.Error{Op: ue.Op, URL: "", Err: ue.Err}
	}
	if u.RawQuery == "" && u.User == nil {
		return err
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.User = nil
	return &url.Error{Op: ue.Op, URL: u.String(), Err: ue.Err}
}

// requestIDFromHeaders is case-insensitive (Go's http.Header is canonicalized,
// but proxies can return either spelling).
func requestIDFromHeaders(h http.Header) string {
	if v := h.Get("X-Request-Id"); v != "" {
		return v
	}
	return h.Get("X-Request-ID")
}

// Close releases the transport when this httpClient owns it.
func (h *httpClient) Close() error {
	if h.owned {
		// http.Client doesn't expose a Close, but we can close idle
		// connections to release sockets eagerly.
		h.hc.CloseIdleConnections()
	}
	return nil
}

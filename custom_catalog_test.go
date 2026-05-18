package audd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomCatalog_Add_Success(t *testing.T) {
	var seenAudioID string
	var seenFile bool
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/upload/", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenAudioID = r.FormValue("audio_id")
		_, _, err := r.FormFile("file")
		seenFile = err == nil
		_, _ = w.Write([]byte(`{"status":"success","result":true}`))
	})
	defer func() { _ = c.Close() }()

	err := c.CustomCatalog().Add(42, []byte("song-bytes"))
	require.NoError(t, err)
	assert.Equal(t, "42", seenAudioID)
	assert.True(t, seenFile, "must upload a file part")
}

func TestCustomCatalog_Add_AccessError_Override(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":904,"error_message":"no access"}}`))
	})
	defer func() { _ = c.Close() }()

	err := c.CustomCatalog().Add(1, []byte("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomCatalogAccess)
	assert.ErrorIs(t, err, ErrSubscription)

	var ccErr *AudDCustomCatalogAccessError
	require.True(t, errors.As(err, &ccErr))
	assert.Contains(t, err.Error(), "custom catalog")
	assert.Contains(t, err.Error(), "api@audd.io")
	assert.Contains(t, err.Error(), "no access")
}

func TestCustomCatalog_Add_OtherErrorIsNotOverridden(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":900,"error_message":"bad token"}}`))
	})
	defer func() { _ = c.Close() }()
	err := c.CustomCatalog().Add(1, []byte("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)
	var ccErr *AudDCustomCatalogAccessError
	assert.False(t, errors.As(err, &ccErr))
}

func TestCustomCatalog_Add_ReaderSource(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, _, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})
	defer func() { _ = c.Close() }()
	require.NoError(t, c.CustomCatalog().Add(7, strings.NewReader("buf")))
}

// newCustomCatalogClientWithBigRetry builds a client whose user-configured
// MaxAttempts is 5 — enough to prove that CustomCatalog.Add overrides the
// global setting and pins to a single attempt. Upload is metered, so retry
// could double-charge.
func newCustomCatalogClientWithBigRetry(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("test", WithHTTPClient(srv.Client()), WithMaxAttempts(5))
	c.standardHTTP = newHTTPClient("test", 0, srv.Client())
	c.enterpriseHTTP = newHTTPClient("test", 0, srv.Client())
	c.standardHTTP.apiToken = "test-token"
	c.enterpriseHTTP.apiToken = "test-token"
	c.standardHTTP.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	c.enterpriseHTTP.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	return c
}

func TestCustomCatalog_Add_DoesNotRetryOn5xx(t *testing.T) {
	var calls int32
	c := newCustomCatalogClientWithBigRetry(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	})
	defer func() { _ = c.Close() }()

	err := c.CustomCatalog().Add(1, []byte("audio"))
	require.Error(t, err, "5xx must surface as an error")
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "metered upload must not be retried on 5xx (could double-charge)")
}

func TestCustomCatalog_AddContext_DoesNotRetryOn5xx(t *testing.T) {
	var calls int32
	c := newCustomCatalogClientWithBigRetry(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	})
	defer func() { _ = c.Close() }()

	err := c.CustomCatalog().AddContext(context.Background(), 1, []byte("audio"))
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "metered upload must not be retried on 5xx (could double-charge)")
}

// dialErrorTransport returns a pre-upload connect error on every RoundTrip,
// without ever sending bytes. Pre-upload connect errors are the one class
// that mutating endpoints normally retry — but custom-catalog upload must
// NOT retry, since the server may have already done (and charged for) the
// fingerprinting work on a previous attempt that hung mid-response.
type dialErrorTransport struct {
	calls *int32
}

func (d dialErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(d.calls, 1)
	return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
}

func TestCustomCatalog_Add_DoesNotRetryOnPreUploadConnectError(t *testing.T) {
	var calls int32
	c := NewClient("test", WithMaxAttempts(5))
	t.Cleanup(func() { _ = c.Close() })
	c.standardHTTP.hc = &http.Client{Transport: dialErrorTransport{calls: &calls}}

	err := c.CustomCatalog().Add(1, []byte("audio"))
	require.Error(t, err)
	var connErr *AudDConnectionError
	require.True(t, errors.As(err, &connErr), "transport failure must be wrapped as AudDConnectionError")
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "metered upload must not be retried on transport failure (could double-charge)")
}

func TestCustomCatalog_AddContext_DoesNotRetryOnPreUploadConnectError(t *testing.T) {
	var calls int32
	c := NewClient("test", WithMaxAttempts(5))
	t.Cleanup(func() { _ = c.Close() })
	c.standardHTTP.hc = &http.Client{Transport: dialErrorTransport{calls: &calls}}

	err := c.CustomCatalog().AddContext(context.Background(), 1, []byte("audio"))
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "metered upload must not be retried on transport failure (could double-charge)")
}

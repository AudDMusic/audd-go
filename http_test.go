package audd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostForm_IncludesAPIToken(t *testing.T) {
	var seenToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenToken = r.FormValue("api_token")
		w.Header().Set("X-Request-Id", "req-1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	defer srv.Close()

	h := newHTTPClient("my-token", 5*time.Second, nil)
	resp, err := h.postForm(context.Background(), srv.URL, formFields{Data: map[string]string{"foo": "bar"}})
	require.NoError(t, err)
	assert.Equal(t, "my-token", seenToken)
	assert.Equal(t, 200, resp.HTTPStatus)
	assert.Equal(t, "req-1", resp.RequestID)
	assert.Equal(t, "success", resp.JSONBody["status"])
}

func TestPostForm_FileFieldUploads(t *testing.T) {
	var seenFileBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, _, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		seenFileBytes, _ = io.ReadAll(f)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	h := newHTTPClient("t", 5*time.Second, nil)
	_, err := h.postForm(context.Background(), srv.URL, formFields{
		File: &fileField{Name: "f.bin", ContentType: "application/octet-stream", Reader: strings.NewReader("hello")},
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), seenFileBytes)
}

func TestTransportError_URLCarriesNoQuery(t *testing.T) {
	// A refused connection: the transport error names the URL, which must
	// not carry the query string.
	h := newHTTPClient("tok", 2*time.Second, nil)
	_, err := h.getNoToken(context.Background(), "http://127.0.0.1:1/longpoll/?api_token=SECRET-XYZ",
		map[string]string{"category": "cat", "timeout": "5"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-XYZ")
	assert.NotContains(t, err.Error(), "api_token")
	assert.NotContains(t, err.Error(), "category=")
	assert.Contains(t, err.Error(), "http://127.0.0.1:1/longpoll/")

	var urlErr *url.Error
	require.True(t, errors.As(err, &urlErr), "the *url.Error type is kept")
	assert.True(t, isPreUploadConnectionError(err), "the cause is kept for retry classification")

	wrapped := &AudDConnectionError{Cause: err}
	assert.NotContains(t, wrapped.Error(), "SECRET-XYZ")
}

func TestPostForm_InjectedClientStillBoundedByTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	// An injected client with no Timeout of its own: the SDK's per-request
	// timeout still bounds the call.
	h := newHTTPClient("tok", 100*time.Millisecond, &http.Client{})
	start := time.Now()
	_, err := h.postForm(context.Background(), srv.URL, formFields{Data: map[string]string{"a": "b"}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second)
}

func TestGetNoToken_OmitsAPIToken(t *testing.T) {
	var seenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	h := newHTTPClient("tok", 5*time.Second, nil)
	_, err := h.getNoToken(context.Background(), srv.URL, map[string]string{"category": "cat"})
	require.NoError(t, err)
	assert.NotContains(t, seenURL, "api_token=")
	assert.Contains(t, seenURL, "category=cat")
}

func TestUserAgent_HasSDKAndRuntime(t *testing.T) {
	ua := userAgent()
	assert.Contains(t, ua, "audd-go/")
	assert.Contains(t, ua, "go/")
}

func TestCustomHTTPClient_IsRespected(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	custom := &http.Client{Timeout: 2 * time.Second}
	h := newHTTPClient("t", 0, custom)
	require.False(t, h.owned, "custom client must not be owned by audd")

	_, err := h.postForm(context.Background(), srv.URL, formFields{})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestNonJSONBody_StillParsedToHTTPResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>upstream gateway error</html>"))
	}))
	defer srv.Close()

	h := newHTTPClient("t", 5*time.Second, nil)
	resp, err := h.postForm(context.Background(), srv.URL, formFields{})
	require.NoError(t, err)
	assert.Equal(t, 502, resp.HTTPStatus)
	assert.Nil(t, resp.JSONBody)
	assert.Contains(t, string(resp.RawBody), "<html>")
}

func TestRequestIDHeader_CaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "abc-123")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	h := newHTTPClient("t", 5*time.Second, nil)
	resp, err := h.postForm(context.Background(), srv.URL, formFields{})
	require.NoError(t, err)
	assert.Equal(t, "abc-123", resp.RequestID)
}

func TestPostForm_CtxCancelPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := newHTTPClient("t", 5*time.Second, nil)
	_, err := h.postForm(ctx, srv.URL, formFields{})
	require.Error(t, err)
}

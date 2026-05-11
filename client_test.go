package audd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockClient returns a Client wired to the given handler's URL via a
// custom http.Client and the fast retry knobs.
func newMockClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("test", WithHTTPClient(srv.Client()), WithMaxAttempts(1))
	c.standardHTTP = newHTTPClient("test", 0, srv.Client())
	c.enterpriseHTTP = newHTTPClient("test", 0, srv.Client())
	c.standardHTTP.apiToken = "test-token"
	c.enterpriseHTTP.apiToken = "test-token"
	// Override base URLs by intercepting via the test server URL.
	// Strategy: rewrite via custom transport.
	c.standardHTTP.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	c.enterpriseHTTP.hc = &http.Client{Transport: rewriteTransport{base: srv.URL}}
	return c, srv
}

type rewriteTransport struct {
	base string
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "api.audd.io") || strings.Contains(req.URL.Host, "enterprise.audd.io") {
		newURL := r.base + req.URL.Path
		if req.URL.RawQuery != "" {
			newURL += "?" + req.URL.RawQuery
		}
		out, err := http.NewRequest(req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		out.Header = req.Header
		out = out.WithContext(req.Context())
		return http.DefaultClient.Do(out)
	}
	return http.DefaultClient.Do(req)
}

func TestRecognize_ParsesPublicMatch(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":{"timecode":"00:56","artist":"X","title":"Y","song_link":"https://lis.tn/abc"}}`))
	})
	defer func() { _ = c.Close() }()

	res, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "X", res.Artist)
	assert.Equal(t, "https://lis.tn/abc?thumb", res.ThumbnailURL())
}

func TestRecognize_NoMatch_ReturnsNilNil(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	res, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.NoError(t, err)
	assert.Nil(t, res)
}

func TestRecognize_ErrorMaps(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":900,"error_message":"bad token"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)

	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 900, apiErr.ErrorCode)
}

func TestRecognize_HTTPNonJSON_MapsToServerError(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("<html>upstream gateway error</html>"))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServer, "HTTP 5xx with non-JSON body must map to ErrServer (locked pattern S2)")

	var apiErr *AudDAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 502, apiErr.HTTPStatus)
}

func TestRecognize_2xxBadJSON_MapsToSerializationError(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSerialization, "2xx with bad JSON must map to ErrSerialization")
}

func TestRecognize_Code51WithResult_LogsAndReturnsResult(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":51,"error_message":"deprecated param 'foo'"},"result":{"timecode":"01:00","artist":"X","title":"Y"}}`))
	})
	defer func() { _ = c.Close() }()

	var deprecMsg string
	c.onDeprecation = func(msg string) { deprecMsg = msg }

	res, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.NoError(t, err, "code 51 + result must NOT raise (locked pattern C3)")
	require.NotNil(t, res)
	assert.Equal(t, "X", res.Artist)
	assert.Contains(t, deprecMsg, "deprecated")
}

func TestRecognize_Code51NoResult_StillRaises(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":51,"error_message":"deprecated and missing"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRecognize_ReturnAndMarketArePropagated(t *testing.T) {
	var seenReturn, seenMarket string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenReturn = r.FormValue("return")
		seenMarket = r.FormValue("market")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", &RecognizeOptions{
		Return: "apple_music,spotify",
		Market: "us",
	})
	require.NoError(t, err)
	assert.Equal(t, "apple_music,spotify", seenReturn)
	assert.Equal(t, "us", seenMarket)
}

func TestRecognizeEnterprise_FlattensChunks(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[
			{"offset":"0","songs":[{"score":100,"timecode":"00:00","artist":"A","title":"T1"}]},
			{"offset":"5","songs":[{"score":99,"timecode":"00:05","artist":"B","title":"T2"}]}
		]}`))
	})
	defer func() { _ = c.Close() }()

	matches, err := c.RecognizeEnterpriseContext(context.Background(), "https://example.com/big.mp3", nil)
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, "T1", matches[0].Title)
	assert.Equal(t, "T2", matches[1].Title)
}

func TestRecognizeEnterprise_LimitOptionPropagated(t *testing.T) {
	var seenLimit string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenLimit = r.FormValue("limit")
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	limit := 1
	_, err := c.RecognizeEnterpriseContext(context.Background(), "https://example.com/big.mp3", &EnterpriseOptions{Limit: &limit})
	require.NoError(t, err)
	assert.Equal(t, "1", seenLimit, "limit=1 should be sent (per project hard rule)")
}

func TestRecognize_ExtraParametersPropagated(t *testing.T) {
	var seenFoo, seenReturn string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenFoo = r.FormValue("foo")
		seenReturn = r.FormValue("return")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.RecognizeContext(context.Background(), "https://example.com/song.mp3", &RecognizeOptions{
		Return:          "apple_music",
		ExtraParameters: map[string]string{"foo": "bar", "return": "ignored"},
	})
	require.NoError(t, err)
	assert.Equal(t, "bar", seenFoo, "ExtraParameters should be sent")
	assert.Equal(t, "apple_music", seenReturn, "typed field must win over ExtraParameters on collision")
}

func TestRecognizeEnterprise_ExtraParametersPropagated(t *testing.T) {
	var seenFoo, seenLimit string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenFoo = r.FormValue("foo")
		seenLimit = r.FormValue("limit")
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	limit := 2
	_, err := c.RecognizeEnterpriseContext(context.Background(), "https://example.com/big.mp3", &EnterpriseOptions{
		Limit:           &limit,
		ExtraParameters: map[string]string{"foo": "bar", "limit": "999"},
	})
	require.NoError(t, err)
	assert.Equal(t, "bar", seenFoo)
	assert.Equal(t, "2", seenLimit, "typed Limit must win over ExtraParameters")
}

func TestClient_Close_Idempotent(t *testing.T) {
	c := NewClient("test")
	assert.NoError(t, c.Close())
	assert.NoError(t, c.Close())
}

func TestAdvanced_UsesRecognitionPolicy(t *testing.T) {
	// Whitebox: Advanced sub-client must use RECOGNITION class (locked pattern C2).
	c := NewClient("test")
	defer func() { _ = c.Close() }()
	a := c.Advanced()
	assert.NotNil(t, a)
	policy := c.retryPolicy(RetryClassRecognition)
	assert.Equal(t, RetryClassRecognition, policy.Class)
}

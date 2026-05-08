package audd

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvanced_FindLyrics_ParsesArray(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/findLyrics/", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"success","result":[
			{"artist":"A","title":"T","lyrics":"la la"}
		]}`))
	})
	defer func() { _ = c.Close() }()

	out, err := c.Advanced().FindLyrics("rule the world")
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "A", out[0].Artist)
	assert.Equal(t, "la la", out[0].Lyrics)
}

func TestAdvanced_FindLyrics_NoResultsIsEmptySlice(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","result":[]}`))
	})
	defer func() { _ = c.Close() }()

	out, err := c.Advanced().FindLyrics("x")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestAdvanced_FindLyrics_ErrorMaps(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"error_code":900,"error_message":"bad"}}`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Advanced().FindLyrics("x")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestAdvanced_RawRequest_PassThroughBody(t *testing.T) {
	var seenMethod, seenQ string
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.URL.Path
		require.NoError(t, r.ParseMultipartForm(1<<20))
		seenQ = r.FormValue("q")
		_, _ = w.Write([]byte(`{"status":"success","result":{"x":1}}`))
	})
	defer func() { _ = c.Close() }()

	body, err := c.Advanced().RawRequest("findLyrics", map[string]string{"q": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "/findLyrics/", seenMethod)
	assert.Equal(t, "hello", seenQ)
	assert.Equal(t, "success", body["status"])
}

func TestAdvanced_RawRequest_NonJSONErrorMaps(t *testing.T) {
	c, _ := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte(`<html>x</html>`))
	})
	defer func() { _ = c.Close() }()

	_, err := c.Advanced().RawRequest("findLyrics", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServer)
}

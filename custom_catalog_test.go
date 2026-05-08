package audd

import (
	"errors"
	"net/http"
	"strings"
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

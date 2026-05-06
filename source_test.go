package audd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSource_URL(t *testing.T) {
	r, err := prepareSource("https://example.com/song.mp3")
	require.NoError(t, err)
	fields, err := r()
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/song.mp3", fields.Data["url"])
	assert.Nil(t, fields.File)

	// Re-open is fresh each time.
	fields2, err := r()
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/song.mp3", fields2.Data["url"])
}

func TestPrepareSource_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "song.bin")
	require.NoError(t, os.WriteFile(path, []byte("hello-bytes"), 0o600))

	r, err := prepareSource(path)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		fields, err := r()
		require.NoError(t, err, "attempt %d", i)
		require.NotNil(t, fields.File)
		got, err := io.ReadAll(fields.File.Reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello-bytes"), got, "attempt %d should re-open file", i)
		if c, ok := fields.File.Reader.(io.Closer); ok {
			_ = c.Close()
		}
	}
}

func TestPrepareSource_Bytes(t *testing.T) {
	r, err := prepareSource([]byte("buf-1"))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		fields, err := r()
		require.NoError(t, err)
		got, err := io.ReadAll(fields.File.Reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("buf-1"), got, "attempt %d", i)
	}
}

func TestPrepareSource_SeekableReader_Retries(t *testing.T) {
	r, err := prepareSource(strings.NewReader("seek-me"))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		fields, err := r()
		require.NoError(t, err, "attempt %d", i)
		got, err := io.ReadAll(fields.File.Reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("seek-me"), got)
	}
}

type unseeker struct{ r io.Reader }

func (u *unseeker) Read(p []byte) (int, error) { return u.r.Read(p) }

func TestPrepareSource_UnseekableReader_FailsOnRetry(t *testing.T) {
	r, err := prepareSource(&unseeker{r: bytes.NewBufferString("once")})
	require.NoError(t, err)
	_, err = r()
	require.NoError(t, err)
	_, err = r()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unseekable")
}

func TestPrepareSource_TypodURL_GivesBetterError(t *testing.T) {
	_, err := prepareSource("htps://example.com/song.mp3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an HTTP URL")
	assert.Contains(t, err.Error(), "is not an existing file path")
}

func TestPrepareSource_RejectsUnknownType(t *testing.T) {
	_, err := prepareSource(123)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source type")
}

func TestPrepareSource_RejectsNil(t *testing.T) {
	_, err := prepareSource(nil)
	require.Error(t, err)
}

func TestPrepareSource_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := prepareSource(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an HTTP URL")
}

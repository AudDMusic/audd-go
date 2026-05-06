package audd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Source is what callers pass to Recognize / RecognizeEnterprise. Acceptable
// concrete types are:
//
//   - string — an HTTP(S) URL or an existing file path on disk.
//   - []byte — raw bytes; safe to retry.
//   - io.Reader — caller's choice. Must be an io.Seeker if retries are
//     possible; non-seekers will surface a clean error on attempt 2 rather
//     than silently sending an empty body.
//
// (We don't define this as a closed sum type because Go interfaces don't
// constrain that way; runtime type-switches in prepareSource accept the
// supported set and reject everything else.)
type Source any

// reopenerFn yields fresh request fields on each invocation. Used inside the
// retry-wrapped request closure so each attempt sends a fresh body.
type reopenerFn func() (formFields, error)

// prepareSource returns a re-opener for the given source.
//
// Per-attempt re-opening is a defensive pattern: net/http does NOT auto-rewind
// a body reader between attempts, so without this we'd silently send empty
// bodies on retry. Mandatory across the SDK family for consistency and safety.
func prepareSource(source Source) (reopenerFn, error) {
	switch s := source.(type) {
	case string:
		return prepareStringSource(s)
	case []byte:
		buf := s
		return func() (formFields, error) {
			return formFields{File: &fileField{
				Name: "upload.bin", ContentType: "application/octet-stream",
				Reader: bytes.NewReader(buf),
			}}, nil
		}, nil
	case io.Reader:
		return prepareReaderSource(s)
	case nil:
		return nil, fmt.Errorf("audd: source must not be nil")
	default:
		return nil, fmt.Errorf("audd: unsupported source type %T; pass a URL string, a file path string, an io.Reader, or []byte", source)
	}
}

// prepareStringSource handles the URL / file-path / typo'd-input cases.
func prepareStringSource(s string) (reopenerFn, error) {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		urlStr := s
		return func() (formFields, error) {
			return formFields{Data: map[string]string{"url": urlStr}}, nil
		}, nil
	}
	// File path?
	if info, err := os.Stat(s); err == nil && !info.IsDir() {
		path := s
		return func() (formFields, error) {
			f, err := os.Open(path) // nolint:gosec // user-supplied path is intentional
			if err != nil {
				return formFields{}, err
			}
			return formFields{File: &fileField{
				Name: filepath.Base(path), ContentType: "application/octet-stream",
				Reader: f,
			}}, nil
		}, nil
	}
	// Better message for typo'd URLs. Tell them what we were looking for
	// instead of just "file not found".
	return nil, fmt.Errorf("audd: %q is not an HTTP URL (must start with http:// or https://) and is not an existing file path; pass a URL string, a file path string, an io.Reader, or []byte", s)
}

// prepareReaderSource handles io.Reader sources, with a one-shot guard that
// fails clearly if a non-seeker is retried.
func prepareReaderSource(r io.Reader) (reopenerFn, error) {
	seeker, isSeeker := r.(io.Seeker)
	var startPos int64
	if isSeeker {
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			isSeeker = false
		} else {
			startPos = pos
		}
	}
	first := true
	return func() (formFields, error) {
		if first {
			first = false
		} else {
			if !isSeeker {
				return formFields{}, errors.New("audd: cannot retry an unseekable io.Reader; pass []byte (buffer the content yourself) or a file path / URL instead")
			}
			if _, err := seeker.Seek(startPos, io.SeekStart); err != nil {
				return formFields{}, err
			}
		}
		return formFields{File: &fileField{
			Name: "upload.bin", ContentType: "application/octet-stream",
			Reader: r,
		}}, nil
	}, nil
}

package audd

import (
	"net/http"
)

// Test helpers shared across the small focused test files in this package.
// They use the existing rewriteTransport pattern (see client_test.go) to
// point a Client at an httptest.NewServer URL — the difference vs.
// newMockClient is that these helpers wrap NewClient so options like
// WithOnEvent or WithMaxAttempts compose naturally.

func mkPolishClient(token, baseURL string) *Client {
	c := NewClient(token, WithMaxAttempts(1))
	c.standardHTTP = newHTTPClient(token, 0, &http.Client{Transport: rewriteTransport{base: baseURL}})
	c.enterpriseHTTP = newHTTPClient(token, 0, &http.Client{Transport: rewriteTransport{base: baseURL}})
	return c
}

func mkPolishClientWithEvent(token, baseURL string, hook OnEventHook) *Client {
	c := NewClient(token, WithMaxAttempts(1), WithOnEvent(hook))
	c.standardHTTP = newHTTPClient(token, 0, &http.Client{Transport: rewriteTransport{base: baseURL}})
	c.enterpriseHTTP = newHTTPClient(token, 0, &http.Client{Transport: rewriteTransport{base: baseURL}})
	return c
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

package audd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

// Tests for the API-token surface: env-var auto-pickup (AUDD_API_TOKEN),
// NewClientStrict's missing-token error, and the thread-safe SetAPIToken
// rotation hot-swap.

func TestEnvVarSuppliesToken(t *testing.T) {
	prev := os.Getenv(tokenEnvVar)
	t.Cleanup(func() { _ = os.Setenv(tokenEnvVar, prev) })
	_ = os.Setenv(tokenEnvVar, "from-env")
	c := NewClient("")
	if got := c.APIToken(); got != "from-env" {
		t.Fatalf("expected 'from-env', got %q", got)
	}
}

func TestExplicitTokenWinsOverEnv(t *testing.T) {
	prev := os.Getenv(tokenEnvVar)
	t.Cleanup(func() { _ = os.Setenv(tokenEnvVar, prev) })
	_ = os.Setenv(tokenEnvVar, "from-env")
	c := NewClient("explicit")
	if got := c.APIToken(); got != "explicit" {
		t.Fatalf("expected 'explicit', got %q", got)
	}
}

func TestNewClientStrictReturnsErrorWhenMissing(t *testing.T) {
	prev := os.Getenv(tokenEnvVar)
	t.Cleanup(func() { _ = os.Setenv(tokenEnvVar, prev) })
	_ = os.Unsetenv(tokenEnvVar)
	_, err := NewClientStrict("")
	if !errors.Is(err, ErrMissingAPIToken) {
		t.Fatalf("expected ErrMissingAPIToken, got %v", err)
	}
}

func TestSetAPITokenRotates(t *testing.T) {
	captured := []string{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		mu.Lock()
		captured = append(captured, r.FormValue("api_token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)

	c := mkPolishClient("t-old", srv.URL)
	if _, err := c.RecognizeContext(context.Background(), "https://x.mp3", nil); err != nil {
		t.Fatalf("recognize 1: %v", err)
	}
	if err := c.SetAPIToken("t-new"); err != nil {
		t.Fatalf("SetAPIToken: %v", err)
	}
	if _, err := c.RecognizeContext(context.Background(), "https://x.mp3", nil); err != nil {
		t.Fatalf("recognize 2: %v", err)
	}
	if !sliceEqual(captured, []string{"t-old", "t-new"}) {
		t.Fatalf("expected [t-old, t-new], got %v", captured)
	}
}

func TestSetAPITokenRejectsEmpty(t *testing.T) {
	c := NewClient("t")
	if err := c.SetAPIToken(""); err == nil {
		t.Fatalf("expected error on empty token")
	}
}

func TestSetAPITokenConcurrent(t *testing.T) {
	// Race rotation against many concurrent recognize() calls. Build with
	// `go test -race`; assertion is just "no panic and all observed tokens
	// are valid" (either old or new).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)

	c := mkPolishClient("t0", srv.URL)
	var done atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.RecognizeContext(context.Background(), "https://x.mp3", nil)
			done.Add(1)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.SetAPIToken("t1")
		_ = c.SetAPIToken("t2")
	}()
	wg.Wait()
	if done.Load() < 20 {
		t.Fatalf("not all goroutines completed: %d", done.Load())
	}
}

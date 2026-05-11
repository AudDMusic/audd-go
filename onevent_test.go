package audd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Tests for the OnEvent inspection hook: emits request/response/exception
// frames, swallows hook panics so observability never breaks the request
// path, and never leaks the api_token into emitted events.

func TestOnEventEmitsRequestThenResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "rid-42")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)

	var events []AudDEvent
	var mu sync.Mutex
	c := mkPolishClientWithEvent("t", srv.URL, func(e AudDEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	if _, err := c.RecognizeContext(context.Background(), "https://x.mp3", nil); err != nil {
		t.Fatalf("recognize: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	if !sliceContains(kinds, "request") {
		t.Errorf("expected 'request' kind, got %v", kinds)
	}
	if !sliceContains(kinds, "response") {
		t.Errorf("expected 'response' kind, got %v", kinds)
	}
	for _, e := range events {
		if e.Kind == "response" && e.RequestID != "rid-42" {
			t.Errorf("expected request id rid-42, got %q", e.RequestID)
		}
	}
}

func TestOnEventHookPanicSwallowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)
	c := mkPolishClientWithEvent("t", srv.URL, func(_ AudDEvent) {
		panic("hook exploded")
	})
	if _, err := c.RecognizeContext(context.Background(), "https://x.mp3", nil); err != nil {
		t.Fatalf("expected request to succeed despite hook panic, got %v", err)
	}
}

func TestOnEventNeverCarriesAPIToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","result":null}`))
	}))
	t.Cleanup(srv.Close)
	const secret = "secret-token-do-not-leak"
	var events []AudDEvent
	var mu sync.Mutex
	c := mkPolishClientWithEvent(secret, srv.URL, func(e AudDEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	_, _ = c.RecognizeContext(context.Background(), "https://x.mp3", nil)
	mu.Lock()
	defer mu.Unlock()
	for _, e := range events {
		blob, _ := json.Marshal(e)
		if strings.Contains(string(blob), secret) {
			t.Fatalf("token leaked into AudDEvent: %s", blob)
		}
	}
}

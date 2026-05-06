// Bridge the SDK's OnEvent hook to log/slog so every request, response,
// and exception lands as a structured JSON record on stdout.
//
//	go run examples/observability_slog/main.go
//
// Output is one JSON line per event — drop it into any log aggregator
// (Datadog, Loki, ELK, GCP Cloud Logging, etc.) without further parsing.
package main

import (
	"fmt"
	"log/slog"
	"os"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	// Configure a JSON slog handler at LevelDebug so nothing the SDK
	// emits is filtered out. Swap NewJSONHandler for NewTextHandler
	// during local development if you prefer human-readable output.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Translate AuddEvent → slog.LogAttrs. The SDK guarantees the event
	// never carries the api_token or request body bytes, so it is safe to
	// forward verbatim into your logging pipeline.
	onEvent := func(e audd.AuddEvent) {
		attrs := []slog.Attr{
			slog.String("kind", e.Kind),
			slog.String("method", e.Method),
			slog.String("url", e.URL),
			slog.Duration("elapsed", e.Elapsed),
		}
		// Optional fields — only attach when populated to keep records tidy.
		if e.RequestID != "" {
			attrs = append(attrs, slog.String("request_id", e.RequestID))
		}
		if e.HTTPStatus != 0 {
			attrs = append(attrs, slog.Int("http_status", e.HTTPStatus))
		}
		if e.ErrorCode != 0 {
			attrs = append(attrs, slog.Int("error_code", e.ErrorCode))
		}
		for k, v := range e.Extras {
			attrs = append(attrs, slog.Any(k, v))
		}

		// Map the event Kind to a slog level: request/response are routine
		// info, exception is a real error worth surfacing on dashboards.
		level := slog.LevelInfo
		msg := "audd." + e.Kind
		if e.Kind == "exception" {
			level = slog.LevelError
		}
		logger.LogAttrs(nil, level, msg, attrs...)
	}

	// Build the client with the OnEvent hook wired up. The hook is
	// off by default — opting in is a single Option.
	client := audd.NewClient("test", audd.WithOnEvent(onEvent))
	defer func() { _ = client.Close() }()

	// Run a recognition. Each call produces one "request" + one
	// "response" (or "exception") log line, with timing and request_id.
	result, err := client.Recognize("https://audd.tech/example.mp3", nil)
	if err != nil {
		logger.Error("recognize failed", "err", err)
		os.Exit(1)
	}
	if result == nil {
		fmt.Println("no match")
		return
	}
	fmt.Printf("%s — %s\n", result.Artist, result.Title)
}

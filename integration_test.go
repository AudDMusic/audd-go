//go:build integration
// +build integration

// Integration tests hit the live AudD API. They are opt-in via the
// `integration` build tag:
//
//	go test -tags=integration -run Integration -v ./...
//
// AUDD_API_TOKEN env var overrides the public test token (the test token
// is capped at 10 requests).
package audd

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationToken() string {
	if v := os.Getenv("AUDD_API_TOKEN"); v != "" {
		return v
	}
	return "test"
}

func TestIntegration_RecognizeURL(t *testing.T) {
	c := NewClient(integrationToken())
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := c.RecognizeContext(ctx, "https://audd.tech/example.mp3", nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Artist)
	assert.NotEmpty(t, res.Title)
}

func TestIntegration_RecognizeURL_WithReturn(t *testing.T) {
	c := NewClient(integrationToken())
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := c.RecognizeContext(ctx, "https://audd.tech/example.mp3", &RecognizeOptions{
		ReturnMetadata: "apple_music",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotNil(t, res.AppleMusic, "apple_music return must populate")
}

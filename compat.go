package audd

import (
	"context"
	"io"
)

// Backwards-compatibility shims for the existing flat API of github.com/AudDMusic/audd-go.
//
// These wrappers stay until v2.0.0. New code should use the namespaced API:
//
//	client.Recognize(source, *RecognizeOptions)
//	client.Streams().Add(AddStreamRequest{...})
//	client.Advanced().FindLyrics(q)
//
// Each call has a *Context counterpart (e.g. RecognizeContext, AddContext,
// FindLyricsContext) that takes a context.Context as its first argument.
//
// See examples/migration_v0_to_v1/main.go.

// Song is the legacy result-shape alias used by the v0 RecognizeByUrl /
// RecognizeByFile / FindLyrics signatures. Internally it's the same as
// Recognition (and LyricsResult, depending on the call-site).
//
// Deprecated: use Recognition / LyricsResult.
type Song = Recognition

// Audd* type aliases — the typed error and event symbols were renamed to
// AudD* (brand-correct PascalCase) in v1.5.7. The old names remain as
// aliases for code already on v1.5.6 or earlier; they will be removed in
// v2.0.0. Update to the AudD* names at your convenience.
//
// Deprecated: use AudDAPIError.
type AuddAPIError = AudDAPIError

// Deprecated: use AudDCustomCatalogAccessError.
type AuddCustomCatalogAccessError = AudDCustomCatalogAccessError

// Deprecated: use AudDConnectionError.
type AuddConnectionError = AudDConnectionError

// Deprecated: use AudDSerializationError.
type AuddSerializationError = AudDSerializationError

// Deprecated: use AudDEvent.
type AuddEvent = AudDEvent

// RecognizeByUrl is the deprecated flat-API wrapper.
//
// Deprecated: use client.Recognize(ctx, url, &RecognizeOptions{ReturnMetadata: ...})
// instead. This shim will be removed in v2.0.0.
func (c *Client) RecognizeByUrl(urlStr, metadata string, options map[string]string) (*Song, error) {
	opts := legacyRecognizeOpts(metadata, options)
	return c.RecognizeContext(context.Background(), urlStr, opts)
}

// RecognizeByFile is the deprecated flat-API wrapper.
//
// Deprecated: use client.Recognize(ctx, reader, &RecognizeOptions{...})
// instead. This shim will be removed in v2.0.0.
func (c *Client) RecognizeByFile(file io.Reader, metadata string, options map[string]string) (*Song, error) {
	opts := legacyRecognizeOpts(metadata, options)
	return c.RecognizeContext(context.Background(), file, opts)
}

// FindLyrics is the deprecated flat-API wrapper.
//
// Deprecated: use client.Advanced().FindLyrics(query) instead. This shim
// will be removed in v2.0.0.
func (c *Client) FindLyrics(query string, _ map[string]string) ([]LyricsResult, error) {
	return c.Advanced().FindLyrics(query)
}

// AddStream is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().Add(AddStreamRequest{...}) instead. This
// shim will be removed in v2.0.0.
func (c *Client) AddStream(streamURL string, radioID int, callbacks string, _ map[string]string) error {
	return c.Streams().Add(AddStreamRequest{
		URL: streamURL, RadioID: radioID, Callbacks: callbacks,
	})
}

// SetCallbackUrl is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().SetCallbackUrl(url, opts) instead. This
// shim will be removed in v2.0.0.
func (c *Client) SetCallbackUrl(callbackURL string) error {
	return c.Streams().SetCallbackUrl(callbackURL, nil)
}

// GetCallbackUrl is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().GetCallbackUrl() instead. This shim will
// be removed in v2.0.0.
func (c *Client) GetCallbackUrl() (string, error) {
	return c.Streams().GetCallbackUrl()
}

// SetStreamUrl is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().SetURL(radioID, url) instead.
func (c *Client) SetStreamUrl(radioID int, urlStr string) error {
	return c.Streams().SetURL(radioID, urlStr)
}

// DeleteStream is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().Delete(radioID) instead.
func (c *Client) DeleteStream(radioID int) error {
	return c.Streams().Delete(radioID)
}

// GetStreams is the deprecated flat-API wrapper.
//
// Deprecated: use client.Streams().List() instead.
func (c *Client) GetStreams() ([]Stream, error) {
	return c.Streams().List()
}

// AddSongToCustomDB is the deprecated flat-API wrapper for CustomCatalog.Add.
//
// Deprecated: use client.CustomCatalog().Add(audioID, source) instead.
func (c *Client) AddSongToCustomDB(audioID int, source any) error {
	return c.CustomCatalog().Add(audioID, source)
}

// legacyRecognizeOpts converts the v0-API "metadata + options map" pair into a
// new-style *RecognizeOptions. The v0 `metadata` arg already matches the
// comma-separated form RecognizeOptions.ReturnMetadata expects, so it passes through
// unchanged.
func legacyRecognizeOpts(metadata string, options map[string]string) *RecognizeOptions {
	opts := &RecognizeOptions{ReturnMetadata: metadata}
	if v, ok := options["market"]; ok {
		opts.Market = v
	}
	return opts
}

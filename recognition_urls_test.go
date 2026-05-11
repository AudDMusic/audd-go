package audd

import (
	"encoding/json"
	"testing"
)

// Tests for the URL helpers on Recognition: StreamingURL, StreamingURLs, and
// PreviewURL — including the lis.tn redirect-form, the metadata-block
// fallbacks (apple_music.url, spotify.external_urls, deezer.link,
// previews/preview_url), and the preference order across providers.

func mkRecognition(songLink string, extras map[string]any) *Recognition {
	r := &Recognition{SongLink: songLink, Extras: map[string]json.RawMessage{}}
	for k, v := range extras {
		b, _ := json.Marshal(v)
		r.Extras[k] = b
	}
	return r
}

func TestStreamingURLLisTnRedirect(t *testing.T) {
	r := mkRecognition("https://lis.tn/abc", nil)
	if got := r.StreamingURL(ProviderSpotify); got != "https://lis.tn/abc?spotify" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := r.StreamingURL(ProviderYouTube); got != "https://lis.tn/abc?youtube" {
		t.Fatalf("youtube unexpected: %q", got)
	}
}

func TestStreamingURLEmptyForYouTubeSongLinkNoMetadata(t *testing.T) {
	r := mkRecognition("https://www.youtube.com/watch?v=x", nil)
	if got := r.StreamingURL(ProviderSpotify); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestStreamingURLFallsBackToAppleMusicURL(t *testing.T) {
	r := mkRecognition("https://www.youtube.com/watch?v=x", map[string]any{
		"apple_music": map[string]any{"url": "https://music.apple.com/us/album/x/123"},
	})
	if got := r.StreamingURL(ProviderAppleMusic); got != "https://music.apple.com/us/album/x/123" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestStreamingURLFallsBackToSpotifyExternalURL(t *testing.T) {
	r := mkRecognition("https://www.youtube.com/watch?v=x", map[string]any{
		"spotify": map[string]any{
			"external_urls": map[string]any{"spotify": "https://open.spotify.com/track/abc"},
		},
	})
	if got := r.StreamingURL(ProviderSpotify); got != "https://open.spotify.com/track/abc" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestStreamingURLPrefersDirectOverLisTnRedirect(t *testing.T) {
	r := mkRecognition("https://lis.tn/abc", map[string]any{
		"apple_music": map[string]any{"url": "https://music.apple.com/us/album/x/123"},
	})
	if got := r.StreamingURL(ProviderAppleMusic); got != "https://music.apple.com/us/album/x/123" {
		t.Fatalf("expected direct URL preference, got %q", got)
	}
}

func TestStreamingURLsUnion(t *testing.T) {
	r := mkRecognition("https://www.youtube.com/watch?v=x", map[string]any{
		"apple_music": map[string]any{"url": "https://music.apple.com/us/album/x/123"},
		"deezer":      map[string]any{"link": "https://www.deezer.com/track/123"},
	})
	urls := r.StreamingURLs()
	if urls[ProviderAppleMusic] != "https://music.apple.com/us/album/x/123" {
		t.Errorf("apple_music: %v", urls)
	}
	if urls[ProviderDeezer] != "https://www.deezer.com/track/123" {
		t.Errorf("deezer: %v", urls)
	}
	if _, ok := urls[ProviderSpotify]; ok {
		t.Errorf("spotify should be absent, got %v", urls)
	}
}

func TestPreviewURLAppleMusicFirst(t *testing.T) {
	r := mkRecognition("", map[string]any{
		"apple_music": map[string]any{
			"previews": []map[string]any{{"url": "https://itunes/preview.m4a"}},
		},
		"spotify": map[string]any{"preview_url": "https://spotify/preview.mp3"},
	})
	if got := r.PreviewURL(); got != "https://itunes/preview.m4a" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestPreviewURLFallsThroughToDeezer(t *testing.T) {
	r := mkRecognition("", map[string]any{
		"deezer": map[string]any{"preview": "https://deezer/preview.mp3"},
	})
	if got := r.PreviewURL(); got != "https://deezer/preview.mp3" {
		t.Fatalf("unexpected: %q", got)
	}
}

package audd

import (
	"encoding/json"
	"net/url"
)

// Forward compatibility: every typed model captures unknown JSON fields into
// an `Extras map[string]json.RawMessage` field plus the full `RawResponse`
// blob. New AudD response keys round-trip through these without an SDK
// release.

// AppleMusicMetadata is the Apple Music metadata block on a recognition.
// All fields are best-effort optional — the AudD payload is rich and changes
// over time.
type AppleMusicMetadata struct {
	ArtistName       string `json:"artistName,omitempty"`
	URL              string `json:"url,omitempty"`
	DurationInMillis int    `json:"durationInMillis,omitempty"`
	Name             string `json:"name,omitempty"`
	ISRC             string `json:"isrc,omitempty"`
	AlbumName        string `json:"albumName,omitempty"`
	TrackNumber      int    `json:"trackNumber,omitempty"`
	ComposerName     string `json:"composerName,omitempty"`
	DiscNumber       int    `json:"discNumber,omitempty"`
	ReleaseDate      string `json:"releaseDate,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// SpotifyMetadata is the Spotify metadata block on a recognition.
type SpotifyMetadata struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	DurationMs  int    `json:"duration_ms,omitempty"`
	Explicit    bool   `json:"explicit,omitempty"`
	Popularity  int    `json:"popularity,omitempty"`
	TrackNumber int    `json:"track_number,omitempty"`
	Type        string `json:"type,omitempty"`
	URI         string `json:"uri,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// DeezerMetadata is the Deezer metadata block.
type DeezerMetadata struct {
	ID       int    `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	Duration int    `json:"duration,omitempty"`
	Link     string `json:"link,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// NapsterMetadata is the Napster metadata block.
type NapsterMetadata struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	ISRC       string `json:"isrc,omitempty"`
	ArtistName string `json:"artistName,omitempty"`
	AlbumName  string `json:"albumName,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// MusicBrainzEntry is one entry in the `musicbrainz` array.
type MusicBrainzEntry struct {
	ID     string          `json:"id"`
	Score  json.RawMessage `json:"score,omitempty"` // server returns int OR string
	Title  string          `json:"title,omitempty"`
	Length int             `json:"length,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// Recognition is the typed result of Client.Recognize. Public-DB matches
// populate Artist/Title/etc.; custom-DB matches populate AudioID instead.
// Use IsPublicMatch / IsCustomMatch to discriminate.
type Recognition struct {
	Timecode    string              `json:"timecode"`
	AudioID     *int                `json:"audio_id,omitempty"`
	Artist      string              `json:"artist,omitempty"`
	Title       string              `json:"title,omitempty"`
	Album       string              `json:"album,omitempty"`
	ReleaseDate string              `json:"release_date,omitempty"`
	Label       string              `json:"label,omitempty"`
	SongLink    string              `json:"song_link,omitempty"`
	ISRC        string              `json:"isrc,omitempty"`
	UPC         string              `json:"upc,omitempty"`
	AppleMusic  *AppleMusicMetadata `json:"apple_music,omitempty"`
	Spotify     *SpotifyMetadata    `json:"spotify,omitempty"`
	Deezer      *DeezerMetadata     `json:"deezer,omitempty"`
	Napster     *NapsterMetadata    `json:"napster,omitempty"`
	MusicBrainz []MusicBrainzEntry  `json:"musicbrainz,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// recognitionKnownKeys is the canonical-keys list used by extras-extraction.
// Update when adding a new typed field on Recognition.
var recognitionKnownKeys = map[string]bool{
	"timecode": true, "audio_id": true, "artist": true, "title": true,
	"album": true, "release_date": true, "label": true, "song_link": true,
	"isrc": true, "upc": true,
	"apple_music": true, "spotify": true, "deezer": true, "napster": true,
	"musicbrainz": true,
}

// extractExtras returns the raw map of unknown keys not in `known`.
func extractExtras(data []byte, known map[string]bool) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for k, v := range raw {
		if !known[k] {
			out[k] = v
		}
	}
	return out, nil
}

// UnmarshalJSON populates Extras + RawResponse alongside the typed fields.
func (r *Recognition) UnmarshalJSON(data []byte) error {
	type alias Recognition
	if err := json.Unmarshal(data, (*alias)(r)); err != nil {
		return err
	}
	extras, err := extractExtras(data, recognitionKnownKeys)
	if err != nil {
		return err
	}
	r.Extras = extras
	r.RawResponse = append(r.RawResponse[:0], data...)
	return nil
}

// IsCustomMatch reports whether this is a custom-DB match (audio_id populated).
func (r *Recognition) IsCustomMatch() bool { return r.AudioID != nil }

// IsPublicMatch reports whether this is a public-DB match (artist/title set,
// audio_id absent).
func (r *Recognition) IsPublicMatch() bool {
	return r.AudioID == nil && (r.Artist != "" || r.Title != "")
}

// ThumbnailURL returns the cover-art URL for lis.tn-hosted song_links, else "".
// YouTube and other hosts return "".
func (r *Recognition) ThumbnailURL() string {
	return lisTnStreamingURL(r.SongLink, "thumb")
}

// StreamingProvider names the streaming services reachable via the lis.tn
// `?<provider>` redirect helper.
type StreamingProvider string

const (
	ProviderSpotify    StreamingProvider = "spotify"
	ProviderAppleMusic StreamingProvider = "apple_music"
	ProviderDeezer     StreamingProvider = "deezer"
	ProviderNapster    StreamingProvider = "napster"
	ProviderYouTube    StreamingProvider = "youtube"
)

// allStreamingProviders is the iteration order for StreamingURLs.
var allStreamingProviders = []StreamingProvider{
	ProviderSpotify, ProviderAppleMusic, ProviderDeezer, ProviderNapster, ProviderYouTube,
}

// lisTnStreamingURL returns "<songLink>?<provider>" only when songLink is on
// lis.tn. Returns "" for non-lis.tn links and when songLink is empty. lis.tn
// 302-redirects "<songLink>?spotify" → that song's Spotify page (etc).
func lisTnStreamingURL(songLink, provider string) string {
	if songLink == "" {
		return ""
	}
	u, err := url.Parse(songLink)
	if err != nil || u.Hostname() != "lis.tn" {
		return ""
	}
	sep := "?"
	if u.RawQuery != "" {
		sep = "&"
	}
	return songLink + sep + provider
}

// directStreamingURL pulls a direct provider URL out of the corresponding
// metadata block when the user requested it via ReturnMetadata. Empty string if not
// available.
func (r *Recognition) directStreamingURL(provider StreamingProvider) string {
	switch provider {
	case ProviderAppleMusic:
		if u, ok := r.Extras["apple_music"]; ok {
			var am struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(u, &am) == nil && am.URL != "" {
				return am.URL
			}
		}
	case ProviderSpotify:
		if u, ok := r.Extras["spotify"]; ok {
			var sp struct {
				ExternalURLs map[string]string `json:"external_urls"`
				URI          string            `json:"uri"`
			}
			if json.Unmarshal(u, &sp) == nil {
				if v := sp.ExternalURLs["spotify"]; v != "" {
					return v
				}
				if sp.URI != "" {
					return sp.URI
				}
			}
		}
	case ProviderDeezer:
		if u, ok := r.Extras["deezer"]; ok {
			var dz struct {
				Link string `json:"link"`
			}
			if json.Unmarshal(u, &dz) == nil && dz.Link != "" {
				return dz.Link
			}
		}
	case ProviderNapster:
		if u, ok := r.Extras["napster"]; ok {
			var np struct {
				Href string `json:"href"`
			}
			if json.Unmarshal(u, &np) == nil && np.Href != "" {
				return np.Href
			}
		}
	}
	return ""
}

// StreamingURL returns a direct or redirect URL for a streaming provider.
//
// Resolution order:
//  1. Direct URL from the metadata block (apple_music.url,
//     spotify.external_urls.spotify, deezer.link, napster.href) when the
//     user requested that provider via ReturnMetadata. Direct = no redirect, faster.
//  2. lis.tn redirect "<SongLink>?<provider>" when SongLink is on lis.tn.
//  3. "" otherwise. YouTube has no metadata-block fallback.
func (r *Recognition) StreamingURL(provider StreamingProvider) string {
	if direct := r.directStreamingURL(provider); direct != "" {
		return direct
	}
	return lisTnStreamingURL(r.SongLink, string(provider))
}

// StreamingURLs returns the union of all providers with a resolvable URL —
// direct or via lis.tn redirect. Empty map when neither path resolves.
func (r *Recognition) StreamingURLs() map[StreamingProvider]string {
	out := map[StreamingProvider]string{}
	for _, p := range allStreamingProviders {
		if u := r.StreamingURL(p); u != "" {
			out[p] = u
		}
	}
	return out
}

// PreviewURL returns the first available 30-second preview URL across
// apple_music.previews[0].url → spotify.preview_url → deezer.preview, in that
// priority order. Empty string if no metadata block carries a preview.
//
// Note: previews are governed by the respective providers' terms of use
// (Apple Music, Spotify, Deezer). The SDK consumer is responsible for honoring
// caching/attribution/redistribution constraints.
func (r *Recognition) PreviewURL() string {
	if am, ok := r.Extras["apple_music"]; ok {
		var amp struct {
			Previews []struct {
				URL string `json:"url"`
			} `json:"previews"`
		}
		if json.Unmarshal(am, &amp) == nil && len(amp.Previews) > 0 && amp.Previews[0].URL != "" {
			return amp.Previews[0].URL
		}
	}
	if sp, ok := r.Extras["spotify"]; ok {
		var sps struct {
			PreviewURL string `json:"preview_url"`
		}
		if json.Unmarshal(sp, &sps) == nil && sps.PreviewURL != "" {
			return sps.PreviewURL
		}
	}
	if dz, ok := r.Extras["deezer"]; ok {
		var dzs struct {
			Preview string `json:"preview"`
		}
		if json.Unmarshal(dz, &dzs) == nil && dzs.Preview != "" {
			return dzs.Preview
		}
	}
	return ""
}

// StreamingURL returns the lis.tn redirect URL for a streaming provider, or
// "" when SongLink is non-lis.tn (EnterpriseMatch doesn't have full metadata
// blocks; only the lis.tn redirect path applies).
func (m *EnterpriseMatch) StreamingURL(provider StreamingProvider) string {
	return lisTnStreamingURL(m.SongLink, string(provider))
}

// StreamingURLs returns all providers with a lis.tn redirect URL available.
func (m *EnterpriseMatch) StreamingURLs() map[StreamingProvider]string {
	out := map[StreamingProvider]string{}
	for _, p := range allStreamingProviders {
		if u := m.StreamingURL(p); u != "" {
			out[p] = u
		}
	}
	return out
}

// EnterpriseMatch is one match within a chunk of the enterprise endpoint's
// response. Multiple matches per file are common.
type EnterpriseMatch struct {
	Score       int    `json:"score"`
	Timecode    string `json:"timecode"`
	Artist      string `json:"artist,omitempty"`
	Title       string `json:"title,omitempty"`
	Album       string `json:"album,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	Label       string `json:"label,omitempty"`
	ISRC        string `json:"isrc,omitempty"`
	UPC         string `json:"upc,omitempty"`
	SongLink    string `json:"song_link,omitempty"`
	StartOffset int    `json:"start_offset,omitempty"`
	EndOffset   int    `json:"end_offset,omitempty"`

	// StartSeconds and EndSeconds locate the match within the uploaded file,
	// in seconds: the chunk's offset in the file plus the fragment-relative
	// StartOffset/EndOffset. nil when the chunk carried no usable offset.
	// Computed by the SDK during flatten, not present on the wire.
	StartSeconds *float64 `json:"-"`
	EndSeconds   *float64 `json:"-"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

var enterpriseMatchKnownKeys = map[string]bool{
	"score": true, "timecode": true, "artist": true, "title": true,
	"album": true, "release_date": true, "label": true, "isrc": true,
	"upc": true, "song_link": true, "start_offset": true, "end_offset": true,
}

// UnmarshalJSON for EnterpriseMatch.
func (m *EnterpriseMatch) UnmarshalJSON(data []byte) error {
	type alias EnterpriseMatch
	if err := json.Unmarshal(data, (*alias)(m)); err != nil {
		return err
	}
	extras, err := extractExtras(data, enterpriseMatchKnownKeys)
	if err != nil {
		return err
	}
	m.Extras = extras
	m.RawResponse = append(m.RawResponse[:0], data...)
	return nil
}

// ThumbnailURL returns the cover-art URL for lis.tn-hosted song_links, else "".
func (m *EnterpriseMatch) ThumbnailURL() string {
	if m.SongLink == "" {
		return ""
	}
	u, err := url.Parse(m.SongLink)
	if err != nil || u.Hostname() != "lis.tn" {
		return ""
	}
	sep := "?"
	if u.RawQuery != "" {
		sep = "&"
	}
	return m.SongLink + sep + "thumb"
}

// EnterpriseChunkResult wraps an array of matches for a single processed
// chunk of the enterprise upload (the response has one element per chunk).
type EnterpriseChunkResult struct {
	Songs  []EnterpriseMatch `json:"songs"`
	Offset string            `json:"offset"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// Stream describes one running stream subscription.
type Stream struct {
	RadioID          int    `json:"radio_id"`
	URL              string `json:"url"`
	StreamRunning    bool   `json:"stream_running"`
	LongpollCategory string `json:"longpoll_category,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

var streamKnownKeys = map[string]bool{
	"radio_id": true, "url": true, "stream_running": true, "longpoll_category": true,
}

func (s *Stream) UnmarshalJSON(data []byte) error {
	type alias Stream
	if err := json.Unmarshal(data, (*alias)(s)); err != nil {
		return err
	}
	extras, err := extractExtras(data, streamKnownKeys)
	if err != nil {
		return err
	}
	s.Extras = extras
	s.RawResponse = append(s.RawResponse[:0], data...)
	return nil
}

// StreamCallbackSong is one candidate song in a recognition match. Almost
// every match has exactly one Song; multiple candidates only appear when the
// same fingerprint resolves to several near-identical catalog records.
type StreamCallbackSong struct {
	Score       int                 `json:"score"`
	Artist      string              `json:"artist"`
	Title       string              `json:"title"`
	Album       string              `json:"album,omitempty"`
	ReleaseDate string              `json:"release_date,omitempty"`
	Label       string              `json:"label,omitempty"`
	SongLink    string              `json:"song_link,omitempty"`
	ISRC        string              `json:"isrc,omitempty"`
	UPC         string              `json:"upc,omitempty"`
	AppleMusic  *AppleMusicMetadata `json:"apple_music,omitempty"`
	Spotify     *SpotifyMetadata    `json:"spotify,omitempty"`
	Deezer      *DeezerMetadata     `json:"deezer,omitempty"`
	Napster     *NapsterMetadata    `json:"napster,omitempty"`
	MusicBrainz []MusicBrainzEntry  `json:"musicbrainz,omitempty"`

	Extras map[string]json.RawMessage `json:"-"`
}

// StreamCallbackMatch is one recognition event from a stream callback or
// longpoll. Carries the top match in Song; rare extra candidates (which may
// have a different artist or title — variant releases, near-duplicates) live
// in Alternatives.
type StreamCallbackMatch struct {
	RadioID    int64  `json:"radio_id"`
	Timestamp  string `json:"timestamp,omitempty"`
	PlayLength int    `json:"play_length,omitempty"`

	Song         StreamCallbackSong   `json:"-"`
	Alternatives []StreamCallbackSong `json:"-"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// StreamCallbackNotification is the lifecycle-event variant of a stream
// callback (e.g. "stream stopped", "can't connect").
type StreamCallbackNotification struct {
	RadioID             int    `json:"radio_id"`
	StreamRunning       *bool  `json:"stream_running,omitempty"`
	NotificationCode    int    `json:"notification_code"`
	NotificationMessage string `json:"notification_message"`
	Time                int    `json:"-"` // outer `time` field (not nested under `notification`)

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

// LyricsResult is one entry in the findLyrics response array.
type LyricsResult struct {
	Artist    string `json:"artist"`
	Title     string `json:"title"`
	Lyrics    string `json:"lyrics,omitempty"`
	SongID    int    `json:"song_id,omitempty"`
	Media     string `json:"media,omitempty"`
	FullTitle string `json:"full_title,omitempty"`
	ArtistID  int    `json:"artist_id,omitempty"`
	SongLink  string `json:"song_link,omitempty"`

	Extras      map[string]json.RawMessage `json:"-"`
	RawResponse json.RawMessage            `json:"-"`
}

var lyricsKnownKeys = map[string]bool{
	"artist": true, "title": true, "lyrics": true, "song_id": true,
	"media": true, "full_title": true, "artist_id": true, "song_link": true,
}

func (l *LyricsResult) UnmarshalJSON(data []byte) error {
	type alias LyricsResult
	if err := json.Unmarshal(data, (*alias)(l)); err != nil {
		return err
	}
	extras, err := extractExtras(data, lyricsKnownKeys)
	if err != nil {
		return err
	}
	l.Extras = extras
	l.RawResponse = append(l.RawResponse[:0], data...)
	return nil
}

// AppleMusic, Spotify, Deezer, Napster, MusicBrainz UnmarshalJSON impls.

var appleMusicKnownKeys = map[string]bool{
	"artistName": true, "url": true, "durationInMillis": true, "name": true,
	"isrc": true, "albumName": true, "trackNumber": true, "composerName": true,
	"discNumber": true, "releaseDate": true,
}

func (a *AppleMusicMetadata) UnmarshalJSON(data []byte) error {
	type alias AppleMusicMetadata
	if err := json.Unmarshal(data, (*alias)(a)); err != nil {
		return err
	}
	extras, err := extractExtras(data, appleMusicKnownKeys)
	if err != nil {
		return err
	}
	a.Extras = extras
	a.RawResponse = append(a.RawResponse[:0], data...)
	return nil
}

var spotifyKnownKeys = map[string]bool{
	"id": true, "name": true, "duration_ms": true, "explicit": true,
	"popularity": true, "track_number": true, "type": true, "uri": true,
}

func (s *SpotifyMetadata) UnmarshalJSON(data []byte) error {
	type alias SpotifyMetadata
	if err := json.Unmarshal(data, (*alias)(s)); err != nil {
		return err
	}
	extras, err := extractExtras(data, spotifyKnownKeys)
	if err != nil {
		return err
	}
	s.Extras = extras
	s.RawResponse = append(s.RawResponse[:0], data...)
	return nil
}

var deezerKnownKeys = map[string]bool{"id": true, "title": true, "duration": true, "link": true}

func (d *DeezerMetadata) UnmarshalJSON(data []byte) error {
	type alias DeezerMetadata
	if err := json.Unmarshal(data, (*alias)(d)); err != nil {
		return err
	}
	extras, err := extractExtras(data, deezerKnownKeys)
	if err != nil {
		return err
	}
	d.Extras = extras
	d.RawResponse = append(d.RawResponse[:0], data...)
	return nil
}

var napsterKnownKeys = map[string]bool{
	"id": true, "name": true, "isrc": true, "artistName": true, "albumName": true,
}

func (n *NapsterMetadata) UnmarshalJSON(data []byte) error {
	type alias NapsterMetadata
	if err := json.Unmarshal(data, (*alias)(n)); err != nil {
		return err
	}
	extras, err := extractExtras(data, napsterKnownKeys)
	if err != nil {
		return err
	}
	n.Extras = extras
	n.RawResponse = append(n.RawResponse[:0], data...)
	return nil
}

var musicBrainzKnownKeys = map[string]bool{"id": true, "score": true, "title": true, "length": true}

func (m *MusicBrainzEntry) UnmarshalJSON(data []byte) error {
	type alias MusicBrainzEntry
	if err := json.Unmarshal(data, (*alias)(m)); err != nil {
		return err
	}
	extras, err := extractExtras(data, musicBrainzKnownKeys)
	if err != nil {
		return err
	}
	m.Extras = extras
	m.RawResponse = append(m.RawResponse[:0], data...)
	return nil
}

var streamCallbackSongKnownKeys = map[string]bool{
	"artist": true, "title": true, "score": true, "album": true,
	"release_date": true, "label": true, "song_link": true,
	"isrc": true, "upc": true,
	"apple_music": true, "spotify": true, "deezer": true, "napster": true,
	"musicbrainz": true,
}

func (s *StreamCallbackSong) UnmarshalJSON(data []byte) error {
	type alias StreamCallbackSong
	if err := json.Unmarshal(data, (*alias)(s)); err != nil {
		return err
	}
	extras, err := extractExtras(data, streamCallbackSongKnownKeys)
	if err != nil {
		return err
	}
	s.Extras = extras
	return nil
}

var streamCallbackMatchKnownKeys = map[string]bool{
	"radio_id": true, "timestamp": true, "play_length": true, "results": true,
}

var streamCallbackNotificationKnownKeys = map[string]bool{
	"radio_id": true, "stream_running": true,
	"notification_code": true, "notification_message": true,
}

func (n *StreamCallbackNotification) UnmarshalJSON(data []byte) error {
	type alias StreamCallbackNotification
	if err := json.Unmarshal(data, (*alias)(n)); err != nil {
		return err
	}
	extras, err := extractExtras(data, streamCallbackNotificationKnownKeys)
	if err != nil {
		return err
	}
	n.Extras = extras
	n.RawResponse = append(n.RawResponse[:0], data...)
	return nil
}

var enterpriseChunkResultKnownKeys = map[string]bool{"songs": true, "offset": true}

func (r *EnterpriseChunkResult) UnmarshalJSON(data []byte) error {
	type alias EnterpriseChunkResult
	if err := json.Unmarshal(data, (*alias)(r)); err != nil {
		return err
	}
	extras, err := extractExtras(data, enterpriseChunkResultKnownKeys)
	if err != nil {
		return err
	}
	r.Extras = extras
	r.RawResponse = append(r.RawResponse[:0], data...)
	return nil
}

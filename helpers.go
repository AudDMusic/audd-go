package audd

import (
	"crypto/md5" // nolint:gosec // AudD's documented longpoll-category formula uses MD5; not crypto-grade.
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DeriveLongpollCategory computes the 9-char longpoll category for a token +
// radio_id. Pure function — no network call.
//
// Formula (per docs.audd.io/streams.md): hex-MD5 of (hex-MD5 of api_token,
// concatenated with the radio_id rendered as a decimal string), truncated
// to the first 9 hex chars.
//
// Use this to share categories with browser/widget code without exposing
// the api_token.
func DeriveLongpollCategory(apiToken string, radioID int) string {
	innerSum := md5.Sum([]byte(apiToken)) // nolint:gosec // see file-level comment
	inner := hex.EncodeToString(innerSum[:])
	outerSum := md5.Sum([]byte(inner + fmt.Sprint(radioID))) // nolint:gosec
	full := hex.EncodeToString(outerSum[:])
	return full[:9]
}

// HandleCallback reads and parses a callback POST body from an *http.Request.
// Closes the request body. Exactly one of (match, notification) is non-nil
// on success; both are nil on error.
//
// Use in your HTTP handler that receives AudD callbacks:
//
//	match, notification, err := audd.HandleCallback(r)
func HandleCallback(r *http.Request) (*StreamCallbackMatch, *StreamCallbackNotification, error) {
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, &AudDConnectionError{Cause: err}
	}
	return ParseCallback(body)
}

// ParseCallback parses a callback POST body into a typed match or
// notification. Recognition callbacks have an outer `result` block;
// notification callbacks have a `notification` block; the discrimination is
// by-key. Exactly one of (match, notification) is non-nil on success.
//
// Prefer HandleCallback when you have an *http.Request — ParseCallback is
// for unusual transports (queue consumers, replay tools, raw bytes from a
// webhook framework).
func ParseCallback(body []byte) (*StreamCallbackMatch, *StreamCallbackNotification, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, &AudDSerializationError{Message: "callback body is not valid JSON: " + err.Error(), RawText: string(body)}
	}

	if notifRaw, ok := raw["notification"]; ok {
		var notif StreamCallbackNotification
		if err := json.Unmarshal(notifRaw, &notif); err != nil {
			return nil, nil, &AudDSerializationError{Message: "callback notification: " + err.Error(), RawText: string(body)}
		}
		notif.RawResponse = append([]byte{}, body...)
		if t, ok := raw["time"]; ok {
			var ts int
			_ = json.Unmarshal(t, &ts)
			notif.Time = ts
		}
		return nil, &notif, nil
	}

	if resultRaw, ok := raw["result"]; ok {
		match, err := parseMatch(resultRaw, body)
		if err != nil {
			return nil, nil, err
		}
		return match, nil, nil
	}

	return nil, nil, &AudDSerializationError{Message: "callback body has neither result nor notification", RawText: string(body)}
}

// parseMatch decodes a `result` block into StreamCallbackMatch. The block's
// `results` array becomes Song (first entry) + Alternatives (remaining).
func parseMatch(resultRaw json.RawMessage, fullBody []byte) (*StreamCallbackMatch, error) {
	type rawMatch struct {
		RadioID    int64                `json:"radio_id"`
		Timestamp  string               `json:"timestamp,omitempty"`
		PlayLength int                  `json:"play_length,omitempty"`
		Results    []StreamCallbackSong `json:"results"`
	}
	var rm rawMatch
	if err := json.Unmarshal(resultRaw, &rm); err != nil {
		return nil, &AudDSerializationError{Message: "callback result: " + err.Error(), RawText: string(fullBody)}
	}
	extras, err := extractExtras(resultRaw, streamCallbackMatchKnownKeys)
	if err != nil {
		return nil, err
	}
	// A successful callback never fails to parse just because `results` is
	// absent or empty: Song stays its zero value and Alternatives is empty.
	var song StreamCallbackSong
	var alternatives []StreamCallbackSong
	if len(rm.Results) > 0 {
		song = rm.Results[0]
		alternatives = rm.Results[1:]
	}
	return &StreamCallbackMatch{
		RadioID:      rm.RadioID,
		Timestamp:    rm.Timestamp,
		PlayLength:   rm.PlayLength,
		Song:         song,
		Alternatives: alternatives,
		Extras:       extras,
		RawResponse:  append([]byte{}, fullBody...),
	}, nil
}

// JoinProviders joins provider names into the comma-separated string
// accepted by RecognizeOptions.ReturnMetadata and SetCallbackUrlOptions.ReturnMetadata.
// Useful when the caller already has a []string in hand from configuration
// or user input.
func JoinProviders(providers ...string) string {
	return strings.Join(providers, ",")
}

// errDuplicateReturnParam is the sentinel returned when a caller passes a
// callback URL that already has a `return` query string AND a non-empty
// ReturnMetadata option — conflicting intent.
var errDuplicateReturnParam = errors.New("audd: callback URL already contains a `return` query parameter; clear ReturnMetadata or remove the parameter from the URL")

// addReturnToURL appends `?return=<csv>` to a callback URL.
//
//   - If `returnMetadata` is empty, returns the URL unchanged.
//   - If the URL already has a `return` query param, returns the typed
//     duplicate-parameter error rather than silently overwriting.
func addReturnToURL(rawURL string, returnMetadata string) (string, error) {
	if returnMetadata == "" {
		return rawURL, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", &AudDAPIError{ErrorCode: 0, Message: "invalid callback URL: " + err.Error()}
	}
	q := parsed.Query()
	if q.Has("return") {
		return "", &AudDAPIError{ErrorCode: 0, Message: errDuplicateReturnParam.Error()}
	}
	q.Set("return", returnMetadata)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// parseOffsetToSeconds parses an enterprise chunk offset into seconds. The
// offset arrives as "SS", "MM:SS", "HH:MM:SS", or a bare number; ok is false
// for anything unparseable. Never panics.
func parseOffsetToSeconds(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if !strings.Contains(s, ":") {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var total float64
	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + v
	}
	return total, true
}

// errAs is a tiny wrapper around errors.As that keeps the call-site readable.
func errAs(err error, target **AudDAPIError) bool {
	return errors.As(err, target)
}

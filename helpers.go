package audd

import (
	"crypto/md5" // nolint:gosec // AudD's documented longpoll-category formula uses MD5; not crypto-grade.
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// ParseCallback parses a callback POST body into a typed payload. Recognition
// callbacks have an outer `result` block; notification callbacks have a
// `notification` block. The discrimination is by-key.
func ParseCallback(body []byte) (*StreamCallbackPayload, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &AuddSerializationError{Message: "callback body is not valid JSON: " + err.Error(), RawText: string(body)}
	}
	out := &StreamCallbackPayload{Raw: append([]byte{}, body...)}

	if notifRaw, ok := raw["notification"]; ok {
		var notif StreamCallbackNotification
		if err := json.Unmarshal(notifRaw, &notif); err != nil {
			return nil, &AuddSerializationError{Message: "callback notification: " + err.Error(), RawText: string(body)}
		}
		out.Notification = &notif
		if t, ok := raw["time"]; ok {
			var ts int
			_ = json.Unmarshal(t, &ts)
			out.Time = ts
		}
		return out, nil
	}
	if resultRaw, ok := raw["result"]; ok {
		var res StreamCallbackResult
		if err := json.Unmarshal(resultRaw, &res); err != nil {
			return nil, &AuddSerializationError{Message: "callback result: " + err.Error(), RawText: string(body)}
		}
		out.Result = &res
		return out, nil
	}
	return nil, &AuddSerializationError{Message: "callback body has neither result nor notification", RawText: string(body)}
}

// errDuplicateReturnParam is the sentinel returned when a caller passes a
// callback URL that already has a `return` query string AND a non-nil
// ReturnMetadata option — conflicting intent.
var errDuplicateReturnParam = errors.New("audd: callback URL already contains a `return` query parameter; pass ReturnMetadata=nil or remove the parameter from the URL")

// addReturnToURL appends `?return=<csv>` to a callback URL.
//
//   - If `returnMetadata` is empty, returns the URL unchanged.
//   - If the URL already has a `return` query param, returns the typed
//     duplicate-parameter error rather than silently overwriting.
func addReturnToURL(rawURL string, returnMetadata []string) (string, error) {
	if len(returnMetadata) == 0 {
		return rawURL, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", &AuddAPIError{ErrorCode: 0, Message: "invalid callback URL: " + err.Error()}
	}
	q := parsed.Query()
	if q.Has("return") {
		return "", &AuddAPIError{ErrorCode: 0, Message: errDuplicateReturnParam.Error()}
	}
	q.Set("return", strings.Join(returnMetadata, ","))
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// errAs is a tiny wrapper around errors.As that keeps the call-site readable.
func errAs(err error, target **AuddAPIError) bool {
	return errors.As(err, target)
}

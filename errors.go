package audd

import (
	"errors"
	"fmt"
)

// Sentinels for errors.Is matching. Each AudD error code maps to one of
// these via *AuddAPIError.Is so callers can write:
//
//	if errors.Is(err, audd.ErrAuthentication) { ... }
//
// while still extracting the typed *AuddAPIError via errors.As.
var (
	ErrAuthentication      = errors.New("audd: authentication error")
	ErrQuota               = errors.New("audd: quota exceeded")
	ErrSubscription        = errors.New("audd: endpoint not enabled on token")
	ErrCustomCatalogAccess = errors.New("audd: custom catalog access denied")
	ErrInvalidRequest      = errors.New("audd: invalid request")
	ErrInvalidAudio        = errors.New("audd: invalid audio")
	ErrRateLimit           = errors.New("audd: rate limit reached")
	ErrStreamLimit         = errors.New("audd: stream slot limit reached")
	ErrNotReleased         = errors.New("audd: song not yet released")
	ErrBlocked             = errors.New("audd: blocked by audd security")
	ErrNeedsUpdate         = errors.New("audd: client update required")
	ErrServer              = errors.New("audd: server error")

	ErrConnection    = errors.New("audd: connection error")
	ErrSerialization = errors.New("audd: serialization error")
)

// AuddAPIError is the typed error returned for any `status: error` response
// from the AudD API. It also covers HTTP non-2xx with a non-JSON body
// (mapped to ErrServer with ErrorCode=0).
//
// Use errors.As to extract:
//
//	var apiErr *audd.AuddAPIError
//	if errors.As(err, &apiErr) {
//	    fmt.Println(apiErr.ErrorCode, apiErr.Message)
//	}
type AuddAPIError struct {
	// ErrorCode is AudD's numeric code (e.g. 900, 904). 0 indicates an HTTP-level
	// failure with no JSON body.
	ErrorCode int
	// Message is the server's human-readable message.
	Message string
	// HTTPStatus is the HTTP response status, if any.
	HTTPStatus int
	// RequestID is the X-Request-Id header from the response, if present.
	RequestID string
	// RequestedParams is the server's redacted echo of the request fields.
	RequestedParams map[string]any
	// RequestMethod is the server's `request_api_method` field (informational).
	RequestMethod string
	// BrandedMessage is the artist/title text from a branded denial response
	// (e.g. abuse-prevention "Sorry, your IP was banned"). Empty when absent.
	// Surfaced on the error, never as a recognition.
	BrandedMessage string
	// RawResponse is the unparsed response body (string or map[string]any).
	RawResponse any
}

func (e *AuddAPIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("[#%d] %s", e.ErrorCode, e.Message)
}

// Is allows errors.Is(err, audd.ErrAuthentication) etc. to match the right
// sentinel for the underlying AudD error code.
func (e *AuddAPIError) Is(target error) bool {
	if e == nil {
		return false
	}
	return target == sentinelForCode(e.ErrorCode)
}

// sentinelForCode maps an AudD numeric code to its sentinel error.
func sentinelForCode(code int) error {
	switch code {
	case 900, 901, 903:
		return ErrAuthentication
	case 902:
		return ErrQuota
	case 904, 905:
		return ErrSubscription
	case 50, 51, 600, 601, 602, 700, 701, 702, 906:
		return ErrInvalidRequest
	case 300, 400, 500:
		return ErrInvalidAudio
	case 610:
		return ErrStreamLimit
	case 611:
		return ErrRateLimit
	case 907:
		return ErrNotReleased
	case 19, 31337:
		return ErrBlocked
	case 20:
		return ErrNeedsUpdate
	default:
		return ErrServer
	}
}

// AuddCustomCatalogAccessError is raised specifically from CustomCatalog.Add
// when the token lacks subscription access (codes 904/905 in that context).
// It carries an overridden user-facing message that disambiguates the
// custom-catalog footgun.
type AuddCustomCatalogAccessError struct {
	*AuddAPIError
	ServerMessage string
}

func (e *AuddCustomCatalogAccessError) Error() string {
	return fmt.Sprintf(`Adding songs to your custom catalog requires enterprise access that isn't enabled on your account.

Note: the custom-catalog endpoint is for adding songs to your private fingerprint database, not for music recognition. If you intended to identify music, use client.Recognize(...) (or client.RecognizeEnterprise(...) for files longer than 25 seconds) instead.

To request custom-catalog access, contact api@audd.io.

[Server message: %s]`, e.ServerMessage)
}

// Is matches both the custom-catalog sentinel and the parent subscription sentinel.
func (e *AuddCustomCatalogAccessError) Is(target error) bool {
	if target == ErrCustomCatalogAccess || target == ErrSubscription {
		return true
	}
	if e.AuddAPIError != nil {
		return e.AuddAPIError.Is(target)
	}
	return false
}

// AuddConnectionError wraps a transport-level failure (DNS, TCP, TLS, timeout).
// errors.Is(err, ErrConnection) returns true.
type AuddConnectionError struct {
	Message string
	Cause   error
}

func (e *AuddConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("audd: connection error: %s", e.Cause)
	}
	return fmt.Sprintf("audd: connection error: %s", e.Message)
}

func (e *AuddConnectionError) Unwrap() error { return e.Cause }
func (e *AuddConnectionError) Is(target error) bool {
	return target == ErrConnection
}

// AuddSerializationError is returned for 2xx HTTP responses whose body is not
// parseable as the expected JSON shape.
type AuddSerializationError struct {
	Message string
	RawText string
}

func (e *AuddSerializationError) Error() string {
	return fmt.Sprintf("audd: serialization error: %s", e.Message)
}

func (e *AuddSerializationError) Is(target error) bool {
	return target == ErrSerialization
}

// brandedMessage extracts a "Artist — Title" string from a result map, if any.
func brandedMessage(result map[string]any) string {
	if result == nil {
		return ""
	}
	artist, _ := result["artist"].(string)
	title, _ := result["title"].(string)
	if artist == "" && title == "" {
		return ""
	}
	if artist != "" && title != "" {
		return artist + " — " + title
	}
	if artist != "" {
		return artist
	}
	return title
}

// asMap returns m as a map[string]any if it is one; otherwise nil.
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// raiseFromErrorResponse inspects a `status=error` body and returns the
// appropriate typed error. `customCatalogContext` flips subscription errors
// to AuddCustomCatalogAccessError.
func raiseFromErrorResponse(body map[string]any, httpStatus int, requestID string, customCatalogContext bool) error {
	errBlock := asMap(body["error"])
	codeF, _ := errBlock["error_code"].(float64)
	code := int(codeF)
	msg, _ := errBlock["error_message"].(string)

	requestedParams := asMap(body["request_params"])
	if requestedParams == nil {
		requestedParams = asMap(body["requested_params"])
	}
	method, _ := body["request_api_method"].(string)
	branded := brandedMessage(asMap(body["result"]))

	apiErr := &AuddAPIError{
		ErrorCode:       code,
		Message:         msg,
		HTTPStatus:      httpStatus,
		RequestID:       requestID,
		RequestedParams: requestedParams,
		RequestMethod:   method,
		BrandedMessage:  branded,
		RawResponse:     body,
	}

	if customCatalogContext && (code == 904 || code == 905) {
		return &AuddCustomCatalogAccessError{AuddAPIError: apiErr, ServerMessage: msg}
	}
	return apiErr
}

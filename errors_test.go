package audd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkErrBody(code int, msg string) map[string]any {
	return map[string]any{
		"status": "error",
		"error":  map[string]any{"error_code": float64(code), "error_message": msg},
	}
}

func TestSentinelMatching_ByCode(t *testing.T) {
	cases := []struct {
		code     int
		sentinel error
	}{
		{900, ErrAuthentication},
		{901, ErrAuthentication},
		{903, ErrAuthentication},
		{902, ErrQuota},
		{904, ErrSubscription},
		{905, ErrSubscription},
		{50, ErrInvalidRequest},
		{51, ErrInvalidRequest},
		{700, ErrInvalidRequest},
		{906, ErrInvalidRequest},
		{300, ErrInvalidAudio},
		{400, ErrInvalidAudio},
		{500, ErrInvalidAudio},
		{610, ErrStreamLimit},
		{611, ErrRateLimit},
		{907, ErrNotReleased},
		{19, ErrBlocked},
		{31337, ErrBlocked},
		{20, ErrNeedsUpdate},
		{100, ErrServer},
		{1000, ErrServer},
		{99999, ErrServer},
	}
	for _, c := range cases {
		err := raiseFromErrorResponse(mkErrBody(c.code, "x"), 200, "", false)
		assert.ErrorIs(t, err, c.sentinel, "code %d", c.code)
	}
}

func TestErrorAs_ExtractsTypedFields(t *testing.T) {
	body := map[string]any{
		"status":             "error",
		"error":              map[string]any{"error_code": float64(900), "error_message": "bad token"},
		"request_params":     map[string]any{"api_token": "d***a"},
		"request_api_method": "recognize",
	}
	err := raiseFromErrorResponse(body, 200, "rid", false)

	var apiErr *AuddAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 900, apiErr.ErrorCode)
	assert.Equal(t, "bad token", apiErr.Message)
	assert.Equal(t, "rid", apiErr.RequestID)
	assert.Equal(t, "recognize", apiErr.RequestMethod)
	assert.Contains(t, apiErr.RequestedParams, "api_token")
}

func TestRequestedParams_AltSpelling(t *testing.T) {
	// AudD endpoints disagree: some use "request_params", some "requested_params".
	body := map[string]any{
		"status":           "error",
		"error":            map[string]any{"error_code": float64(700), "error_message": "no file"},
		"requested_params": map[string]any{"api_token": "x***y"},
	}
	err := raiseFromErrorResponse(body, 200, "", false)
	var apiErr *AuddAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Contains(t, apiErr.RequestedParams, "api_token")
}

func TestBrandedMessage_FromResult(t *testing.T) {
	body := map[string]any{
		"status": "error",
		"error":  map[string]any{"error_code": float64(31337), "error_message": "blocked"},
		"result": map[string]any{
			"artist": "Sorry, your IP was banned",
			"title":  "ApiRequest failed",
		},
	}
	err := raiseFromErrorResponse(body, 200, "", false)
	var apiErr *AuddAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Contains(t, apiErr.BrandedMessage, "Sorry, your IP was banned")
	assert.Contains(t, apiErr.BrandedMessage, "ApiRequest failed")
}

func TestCustomCatalogContext_OverridesMessage(t *testing.T) {
	body := mkErrBody(904, "no subscription")
	err := raiseFromErrorResponse(body, 200, "", true)

	var ccErr *AuddCustomCatalogAccessError
	require.True(t, errors.As(err, &ccErr))

	assert.Contains(t, ccErr.Error(), "custom catalog")
	assert.Contains(t, ccErr.Error(), "api@audd.io")
	assert.Contains(t, ccErr.Error(), "no subscription")
	assert.ErrorIs(t, err, ErrCustomCatalogAccess)
	assert.ErrorIs(t, err, ErrSubscription)
}

func TestCustomCatalogContext_NotAppliedToOtherCodes(t *testing.T) {
	body := mkErrBody(900, "auth")
	err := raiseFromErrorResponse(body, 200, "", true)
	var ccErr *AuddCustomCatalogAccessError
	assert.False(t, errors.As(err, &ccErr))
	assert.ErrorIs(t, err, ErrAuthentication)
}

func TestConnectionError_IsAndUnwrap(t *testing.T) {
	cause := errors.New("dial tcp")
	err := &AuddConnectionError{Message: "x", Cause: cause}
	assert.ErrorIs(t, err, ErrConnection)
	assert.ErrorIs(t, err, cause)
}

func TestSerializationError_Is(t *testing.T) {
	err := &AuddSerializationError{Message: "bad json"}
	assert.ErrorIs(t, err, ErrSerialization)
}

func TestErrorString_FormatsCode(t *testing.T) {
	err := &AuddAPIError{ErrorCode: 900, Message: "bad token"}
	assert.Equal(t, "[#900] bad token", err.Error())
}

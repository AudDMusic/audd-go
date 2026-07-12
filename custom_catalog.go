package audd

import (
	"context"
	"fmt"
	"time"
)

const pathUpload = "/upload/"

// CustomCatalogClient handles uploads to your private fingerprint catalog.
//
// **This is NOT how you submit audio for music recognition.** For
// recognition, use Client.Recognize (or Client.RecognizeEnterprise for files
// longer than 25 seconds). This client manipulates your **private
// fingerprint catalog** so AudD's recognition can later identify *your own*
// tracks for *your account only*. Requires special access — contact
// api@audd.io if you need it enabled.
type CustomCatalogClient struct {
	c *Client
}

func newCustomCatalogClient(c *Client) *CustomCatalogClient {
	return &CustomCatalogClient{c: c}
}

// Add is the no-context convenience wrapper around AddContext. Defaults to
// context.Background().
//
// **Reminder: this is NOT for music recognition.** For recognition, use
// Client.Recognize / Client.RecognizeEnterprise.
func (cc *CustomCatalogClient) Add(audioID int, source Source) error {
	return cc.AddContext(context.Background(), audioID, source)
}

// AddContext fingerprints `source` and stores it under the given audio_id
// slot. Calling again with the same audio_id re-fingerprints that slot.
// There is no public list/delete endpoint; track audio_id ↔ song mappings on
// your side.
//
// **Reminder: this is NOT for music recognition.** For recognition, use
// Client.Recognize / Client.RecognizeEnterprise.
func (cc *CustomCatalogClient) AddContext(ctx context.Context, audioID int, source Source) error {
	reopen, err := prepareSource(source)
	if err != nil {
		return err
	}
	// Custom-catalog upload is metered: every successful call fingerprints
	// audio and is billed. A transport hiccup mid-upload can leave the server
	// having done (and charged for) the work. Retrying could double-charge
	// for the same audio. Pin this endpoint to a single attempt regardless of
	// the client's MaxAttempts override; transient failures surface to the
	// caller as a clean error.
	policy := RetryPolicy{Class: RetryClassMutating, MaxAttempts: 1, BackoffFactor: time.Millisecond}
	endpoint := apiBase + pathUpload
	obs := cc.c.observeCall("upload", endpoint)
	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		fields, fErr := reopen()
		if fErr != nil {
			return nil, fErr
		}
		if fields.Data == nil {
			fields.Data = map[string]string{}
		}
		fields.Data["audio_id"] = fmt.Sprint(audioID)
		return cc.c.standardHTTP.postForm(ctx, endpoint, fields)
	})
	if err != nil {
		obs.fail()
		return &AudDConnectionError{Cause: err}
	}
	obs.done(resp)
	if resp.JSONBody == nil {
		if resp.HTTPStatus >= httpClientErrorFloor {
			return &AudDAPIError{
				ErrorCode:   0,
				Message:     fmt.Sprintf("HTTP %d with non-JSON response body", resp.HTTPStatus),
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: string(resp.RawBody),
			}
		}
		return &AudDSerializationError{Message: "Unparseable response", RawText: string(resp.RawBody)}
	}
	body := resp.JSONBody
	if body["status"] == "error" {
		// custom_catalog_context = true → SubscriptionError → CustomCatalogAccessError.
		return raiseFromErrorResponse(body, resp.HTTPStatus, resp.RequestID, true)
	}
	if body["status"] != "success" {
		return &AudDAPIError{
			ErrorCode:   0,
			Message:     fmt.Sprintf("Unexpected response status: %v", body["status"]),
			HTTPStatus:  resp.HTTPStatus,
			RequestID:   resp.RequestID,
			RawResponse: body,
		}
	}
	return nil
}

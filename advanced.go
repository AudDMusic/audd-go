package audd

import (
	"context"
	"encoding/json"
	"strconv"
)

// AdvancedClient is the escape-hatch namespace: lyrics search and a
// raw-method-name request helper. Reach via Client.Advanced().
//
// This client uses the RECOGNITION retry policy — find_lyrics is metered, so
// we don't double-bill on read timeouts.
type AdvancedClient struct {
	c *Client
}

func newAdvancedClient(c *Client) *AdvancedClient {
	return &AdvancedClient{c: c}
}

// FindLyrics is the no-context convenience wrapper around FindLyricsContext.
// Defaults to context.Background().
func (a *AdvancedClient) FindLyrics(query string) ([]LyricsResult, error) {
	return a.FindLyricsContext(context.Background(), query)
}

// FindLyricsContext searches AudD's lyrics database. Returns an empty slice
// on no match.
func (a *AdvancedClient) FindLyricsContext(ctx context.Context, query string) ([]LyricsResult, error) {
	return a.findLyricsContext(ctx, query, nil)
}

// findLyricsContext is the shared implementation behind FindLyricsContext and
// the v0 compat shim (which forwards its legacy options map as extra params).
func (a *AdvancedClient) findLyricsContext(ctx context.Context, query string, extraParams map[string]string) ([]LyricsResult, error) {
	params := map[string]string{}
	for k, v := range extraParams {
		params[k] = v
	}
	params["q"] = query
	resp, err := a.doRaw(ctx, "findLyrics", params)
	if err != nil {
		return nil, err
	}
	body, err := a.c.decodeOrRaise(resp)
	if err != nil {
		return nil, err
	}
	resultRaw, ok := body["result"]
	if !ok || resultRaw == nil {
		return []LyricsResult{}, nil
	}
	raw, mErr := json.Marshal(resultRaw)
	if mErr != nil {
		return nil, &AudDSerializationError{Message: mErr.Error()}
	}
	var out []LyricsResult
	if uErr := json.Unmarshal(raw, &out); uErr != nil {
		return nil, &AudDSerializationError{Message: uErr.Error(), RawText: string(raw)}
	}
	return out, nil
}

// RawRequest is the no-context convenience wrapper around RawRequestContext.
// Defaults to context.Background().
func (a *AdvancedClient) RawRequest(method string, params map[string]string) (map[string]any, error) {
	return a.RawRequestContext(context.Background(), method, params)
}

// RawRequestContext is the escape hatch for any AudD endpoint not yet wrapped
// by a typed method on this SDK. Hits `https://api.audd.io/<method>/` with
// the given form params and returns the parsed JSON body.
func (a *AdvancedClient) RawRequestContext(ctx context.Context, method string, params map[string]string) (map[string]any, error) {
	resp, err := a.doRaw(ctx, method, params)
	if err != nil {
		return nil, err
	}
	if resp.JSONBody == nil {
		if resp.HTTPStatus >= httpClientErrorFloor {
			return nil, &AudDAPIError{
				ErrorCode:   0,
				Message:     "HTTP " + strconv.Itoa(resp.HTTPStatus) + " with non-JSON response body",
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: string(resp.RawBody),
			}
		}
		return nil, &AudDSerializationError{Message: "Unparseable response", RawText: string(resp.RawBody)}
	}
	return resp.JSONBody, nil
}

// doRaw performs the POST for a raw method name with retries and lifecycle
// events; the caller decides how to decode the response.
func (a *AdvancedClient) doRaw(ctx context.Context, method string, params map[string]string) (*httpResponse, error) {
	policy := a.c.retryPolicy(RetryClassRecognition)
	endpoint := apiBase + "/" + method + "/"
	obs := a.c.observeCall(method, endpoint)
	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		data := map[string]string{}
		for k, v := range params {
			data[k] = v
		}
		return a.c.standardHTTP.postForm(ctx, endpoint, formFields{Data: data})
	})
	if err != nil {
		obs.fail()
		return nil, &AudDConnectionError{Cause: err}
	}
	obs.done(resp)
	return resp, nil
}

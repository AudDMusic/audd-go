package audd

import (
	"context"
	"encoding/json"
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
	body, err := a.RawRequestContext(ctx, "findLyrics", map[string]string{"q": query})
	if err != nil {
		return nil, err
	}
	if body["status"] == "error" {
		return nil, raiseFromErrorResponse(body, 200, "", false)
	}
	resultRaw, ok := body["result"]
	if !ok || resultRaw == nil {
		return []LyricsResult{}, nil
	}
	raw, mErr := json.Marshal(resultRaw)
	if mErr != nil {
		return nil, &AuddSerializationError{Message: mErr.Error()}
	}
	var out []LyricsResult
	if uErr := json.Unmarshal(raw, &out); uErr != nil {
		return nil, &AuddSerializationError{Message: uErr.Error(), RawText: string(raw)}
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
	policy := a.c.retryPolicy(RetryClassRecognition)
	resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		data := map[string]string{}
		for k, v := range params {
			data[k] = v
		}
		return a.c.standardHTTP.postForm(ctx, apiBase+"/"+method+"/", formFields{Data: data})
	})
	if err != nil {
		return nil, &AuddConnectionError{Cause: err}
	}
	if resp.JSONBody == nil {
		if resp.HTTPStatus >= httpClientErrorFloor {
			return nil, &AuddAPIError{
				ErrorCode:   0,
				Message:     "HTTP " + sprintInt(resp.HTTPStatus) + " with non-JSON response body",
				HTTPStatus:  resp.HTTPStatus,
				RequestID:   resp.RequestID,
				RawResponse: string(resp.RawBody),
			}
		}
		return nil, &AuddSerializationError{Message: "Unparseable response", RawText: string(resp.RawBody)}
	}
	return resp.JSONBody, nil
}

// sprintInt is a tiny helper to keep imports tidy.
func sprintInt(i int) string {
	return jsonNumberStr(i)
}

func jsonNumberStr(i int) string {
	// strconv.Itoa via fmt.Sprint to avoid an import flutter on small files.
	return itoa(i)
}

func itoa(i int) string {
	// Inline to avoid pulling strconv in for one call.
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

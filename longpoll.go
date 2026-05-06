package audd

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// longpollURL is the AudD longpoll endpoint. Tokenless — caller authorizes
// the subscription via the category alone.
const longpollURL = "https://api.audd.io/longpoll/"

// LongpollConsumerOption configures a LongpollConsumer.
type LongpollConsumerOption func(*LongpollConsumer)

// LongpollConsumerWithHTTPClient injects a caller-managed *http.Client.
func LongpollConsumerWithHTTPClient(hc *http.Client) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.userHTTPClient = hc }
}

// LongpollConsumerWithMaxAttempts overrides the default retry attempts.
func LongpollConsumerWithMaxAttempts(n int) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.maxAttempts = n }
}

// LongpollConsumerWithBackoffFactor overrides the default initial-backoff.
func LongpollConsumerWithBackoffFactor(d time.Duration) LongpollConsumerOption {
	return func(c *LongpollConsumer) { c.backoffFactor = d }
}

// LongpollConsumer is the tokenless long-polling consumer. Use it from
// browser/widget/extension contexts where you only have a category (and
// no api_token).
//
// Implements io.Closer (Close releases the owned HTTP transport).
//
// Hardening:
//   - HTTP non-2xx → channel emits an event with Err set to *AuddAPIError
//     mapped to ErrServer.
//   - JSON decode failure on a 2xx → emits Err *AuddSerializationError.
//   - READ-class retries on 5xx + connection errors with configurable
//     MaxAttempts / BackoffFactor for parity with the authenticated client.
type LongpollConsumer struct {
	category       string
	userHTTPClient *http.Client
	maxAttempts    int
	backoffFactor  time.Duration

	httpc *httpClient
}

// NewLongpollConsumer builds a tokenless consumer for the given category.
func NewLongpollConsumer(category string, opts ...LongpollConsumerOption) *LongpollConsumer {
	c := &LongpollConsumer{
		category:      category,
		maxAttempts:   3,
		backoffFactor: 500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	c.httpc = newHTTPClient("", defaultStandardTimeout, c.userHTTPClient)
	return c
}

// Close releases the owned HTTP transport. Idempotent.
func (c *LongpollConsumer) Close() error {
	if c.httpc != nil {
		_ = c.httpc.Close()
	}
	return nil
}

// LongpollConsumerOptions controls a single Iterate call.
type LongpollConsumerOptions struct {
	SinceTime int
	Timeout   int
}

// Iterate is the no-context convenience wrapper around IterateContext.
// Defaults to context.Background(). Callers that want to stop the background
// goroutine without draining the channel should use IterateContext with a
// cancellable context.
func (c *LongpollConsumer) Iterate(opts *LongpollConsumerOptions) <-chan LongpollEvent {
	return c.IterateContext(context.Background(), opts)
}

// IterateContext returns a channel of LongpollEvent values. The channel is
// closed when ctx is cancelled or a terminal error occurs.
//
// Each successful poll yields one event with Body set; on error, an event
// with Err set is sent and the channel is closed.
func (c *LongpollConsumer) IterateContext(ctx context.Context, opts *LongpollConsumerOptions) <-chan LongpollEvent {
	if opts == nil {
		opts = &LongpollConsumerOptions{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 50
	}
	out := make(chan LongpollEvent)
	go func() {
		defer close(out)
		curSince := opts.SinceTime
		policy := RetryPolicy{
			Class:         RetryClassRead,
			MaxAttempts:   c.maxAttempts,
			BackoffFactor: c.backoffFactor,
		}
		for {
			params := map[string]string{"category": c.category, "timeout": fmt.Sprint(timeout)}
			if curSince > 0 {
				params["since_time"] = fmt.Sprint(curSince)
			}
			resp, err := retryDo(ctx, policy, func() (*httpResponse, error) {
				return c.httpc.getNoToken(ctx, longpollURL, params)
			})
			if err != nil {
				select {
				case out <- LongpollEvent{Err: &AuddConnectionError{Cause: err}}:
				case <-ctx.Done():
				}
				return
			}
			if resp.HTTPStatus >= httpClientErrorFloor {
				// Non-2xx: surface a typed error and stop. Prevents an
				// untrusted browser widget from spinning forever on a 401.
				apiErr := &AuddAPIError{
					ErrorCode:   0,
					Message:     fmt.Sprintf("Longpoll endpoint returned HTTP %d", resp.HTTPStatus),
					HTTPStatus:  resp.HTTPStatus,
					RequestID:   resp.RequestID,
					RawResponse: stringOrJSON(resp),
				}
				select {
				case out <- LongpollEvent{Err: apiErr}:
				case <-ctx.Done():
				}
				return
			}
			if resp.JSONBody == nil {
				select {
				case out <- LongpollEvent{Err: &AuddSerializationError{
					Message: "Longpoll response was not a JSON object",
					RawText: string(resp.RawBody),
				}}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case out <- LongpollEvent{Body: resp.JSONBody}:
			case <-ctx.Done():
				return
			}
			if ts, ok := resp.JSONBody["timestamp"].(float64); ok {
				curSince = int(ts)
			}
		}
	}()
	return out
}

func stringOrJSON(r *httpResponse) any {
	if r.JSONBody != nil {
		return r.JSONBody
	}
	return string(r.RawBody)
}

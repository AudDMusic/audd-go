package audd

import (
	"context"
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
// goroutine without draining the channels should use IterateContext with a
// cancellable context (or call Close on the returned poll).
func (c *LongpollConsumer) Iterate(opts *LongpollConsumerOptions) *LongpollPoll {
	return c.IterateContext(context.Background(), opts)
}

// IterateContext returns a LongpollPoll whose Matches / Notifications /
// Errors channels are filled by a background goroutine. The poll terminates
// when ctx is cancelled, Close() is called, or a terminal error fires.
//
// Tokenless: no api_token is sent. The category alone authorizes the
// subscription. The user/server who derived the category is responsible for
// ensuring a callback URL is configured on their account (we can't
// preflight that without a token).
func (c *LongpollConsumer) IterateContext(ctx context.Context, opts *LongpollConsumerOptions) *LongpollPoll {
	if opts == nil {
		opts = &LongpollConsumerOptions{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 50
	}
	pollCtx, cancel := context.WithCancel(ctx)
	matches := make(chan StreamCallbackMatch)
	notifs := make(chan StreamCallbackNotification)
	errs := make(chan error, 1)
	policy := RetryPolicy{
		Class:         RetryClassRead,
		MaxAttempts:   c.maxAttempts,
		BackoffFactor: c.backoffFactor,
	}
	go runLongpoll(pollCtx, longpollSource{
		fetch: func(ctx context.Context, params map[string]string) (*httpResponse, error) {
			return retryDo(ctx, policy, func() (*httpResponse, error) {
				return c.httpc.getNoToken(ctx, longpollURL, params)
			})
		},
		category:  c.category,
		sinceTime: opts.SinceTime,
		timeout:   timeout,
	}, matches, notifs, errs)
	return &LongpollPoll{
		Matches:       matches,
		Notifications: notifs,
		Errors:        errs,
		cancel:        cancel,
	}
}

func stringOrJSON(r *httpResponse) any {
	if r.JSONBody != nil {
		return r.JSONBody
	}
	return string(r.RawBody)
}

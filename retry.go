package audd

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"time"
)

// RetryClass determines retry semantics.
//
//	RetryClassRead        — idempotent reads (Streams.List, Streams.GetCallbackUrl):
//	                        retry on 408/429/5xx + any connection error.
//	RetryClassRecognition — Recognize, RecognizeEnterprise, Advanced.FindLyrics:
//	                        retry on pre-upload connection failures + 5xx.
//	                        DO NOT retry on read-timeout-after-upload (cost protection).
//	RetryClassMutating    — Streams.Add, Streams.Delete, etc.:
//	                        retry only on pre-upload connection failures.
//	                        DO NOT retry 5xx (the side effect may have happened).
//	                        NOTE: CustomCatalog.Add does NOT use this class —
//	                        upload is metered, so it pins to 1 attempt to
//	                        avoid double-charging on transient failures.
type RetryClass int

const (
	// RetryClassRead is for idempotent read endpoints.
	RetryClassRead RetryClass = iota
	// RetryClassRecognition is for cost-metered recognition endpoints.
	RetryClassRecognition
	// RetryClassMutating is for endpoints with server-side side effects.
	RetryClassMutating
)

// RetryPolicy controls automatic retries. Zero value means "use defaults"
// when passed to retryDo.
type RetryPolicy struct {
	Class         RetryClass
	MaxAttempts   int           // default 3 (retryDo applies the default if 0)
	BackoffFactor time.Duration // default 500ms; doubles per attempt with jitter
	BackoffMax    time.Duration // default 30s; upper cap on a single delay
}

// defaultRetryPolicy returns a sensible default for the given class.
func defaultRetryPolicy(class RetryClass) RetryPolicy {
	return RetryPolicy{
		Class:         class,
		MaxAttempts:   3,
		BackoffFactor: 500 * time.Millisecond,
		BackoffMax:    30 * time.Second,
	}
}

// withDefaults fills in zero-valued fields with the defaults for the policy's
// class. Callers can override individual fields and leave the rest at zero.
func (p RetryPolicy) withDefaults() RetryPolicy {
	d := defaultRetryPolicy(p.Class)
	if p.MaxAttempts > 0 {
		d.MaxAttempts = p.MaxAttempts
	}
	if p.BackoffFactor > 0 {
		d.BackoffFactor = p.BackoffFactor
	}
	if p.BackoffMax > 0 {
		d.BackoffMax = p.BackoffMax
	}
	return d
}

// backoff returns the next sleep duration. Jitter range is [0.5x, 1.5x] of base.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	base := time.Duration(float64(p.BackoffFactor) * float64(int(1)<<uint(attempt)))
	if base > p.BackoffMax {
		base = p.BackoffMax
	}
	// nolint:gosec // not crypto; jitter only.
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(base) * jitter)
}

const (
	httpRequestTimeout   = 408
	httpTooManyRequests  = 429
	httpServerErrorFloor = 500
)

// shouldRetryResponse decides whether a non-error HTTP response should be retried.
func (p RetryPolicy) shouldRetryResponse(r *httpResponse) bool {
	s := r.HTTPStatus
	switch p.Class {
	case RetryClassRead:
		return s == httpRequestTimeout || s == httpTooManyRequests || s >= httpServerErrorFloor
	case RetryClassRecognition:
		return s >= httpServerErrorFloor
	case RetryClassMutating:
		return false
	}
	return false
}

// isPreUploadConnectionError reports whether `err` happened before the request
// body was sent (DNS, TCP dial, TLS handshake). Cost-protection-critical for
// the recognition class: only THESE retries are safe — post-upload errors
// might have already done metered server work.
func isPreUploadConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// "dial" op = TCP/TLS dial = pre-upload.
		if opErr.Op == "dial" {
			return true
		}
	}
	return false
}

// shouldRetryError decides whether a transport-level error should be retried.
func (p RetryPolicy) shouldRetryError(err error) bool {
	if err == nil {
		return false
	}
	switch p.Class {
	case RetryClassRead:
		// Any net error while issuing the request is fair game for reads.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return true
		}
		var netErr net.Error
		return errors.As(err, &netErr)
	case RetryClassRecognition, RetryClassMutating:
		return isPreUploadConnectionError(err)
	}
	return false
}

// retryDo runs `do` until it succeeds, until policy.MaxAttempts is reached,
// or until ctx is cancelled. The caller's `do` should issue a single HTTP
// request — retryDo handles backoff and re-invocation between attempts.
func retryDo(ctx context.Context, p RetryPolicy, do func() (*httpResponse, error)) (*httpResponse, error) {
	p = p.withDefaults()
	var lastResp *httpResponse
	var lastErr error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		resp, err := do()
		if err != nil {
			lastErr, lastResp = err, nil
			if !p.shouldRetryError(err) {
				return nil, err
			}
		} else {
			lastResp, lastErr = resp, nil
			if !p.shouldRetryResponse(resp) {
				return resp, nil
			}
		}
		if attempt+1 >= p.MaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			if lastResp != nil {
				return lastResp, ctx.Err()
			}
			return nil, ctx.Err()
		case <-time.After(p.backoff(attempt)):
		}
	}
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

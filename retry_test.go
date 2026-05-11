package audd

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fastPolicy(class RetryClass) RetryPolicy {
	return RetryPolicy{Class: class, MaxAttempts: 3, BackoffFactor: time.Microsecond, BackoffMax: time.Millisecond}
}

func TestRetry_Read_Retries5xxAndSucceeds(t *testing.T) {
	calls := 0
	resp, err := retryDo(context.Background(), fastPolicy(RetryClassRead), func() (*httpResponse, error) {
		calls++
		if calls < 3 {
			return &httpResponse{HTTPStatus: 503}, nil
		}
		return &httpResponse{HTTPStatus: 200}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.HTTPStatus)
	assert.Equal(t, 3, calls)
}

func TestRetry_Read_Retries429And408(t *testing.T) {
	for _, status := range []int{408, 429} {
		calls := 0
		resp, err := retryDo(context.Background(), fastPolicy(RetryClassRead), func() (*httpResponse, error) {
			calls++
			if calls < 2 {
				return &httpResponse{HTTPStatus: status}, nil
			}
			return &httpResponse{HTTPStatus: 200}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 200, resp.HTTPStatus)
		assert.Equal(t, 2, calls, "status %d", status)
	}
}

func TestRetry_Read_ExhaustsAndReturnsLast(t *testing.T) {
	calls := 0
	resp, err := retryDo(context.Background(), fastPolicy(RetryClassRead), func() (*httpResponse, error) {
		calls++
		return &httpResponse{HTTPStatus: 503}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 503, resp.HTTPStatus)
	assert.Equal(t, 3, calls)
}

func TestRetry_Mutating_DoesNotRetry5xx(t *testing.T) {
	calls := 0
	resp, err := retryDo(context.Background(), fastPolicy(RetryClassMutating), func() (*httpResponse, error) {
		calls++
		return &httpResponse{HTTPStatus: 503}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 503, resp.HTTPStatus)
	assert.Equal(t, 1, calls, "mutating must not retry 5xx")
}

func TestRetry_Recognition_RetriesOn5xx(t *testing.T) {
	calls := 0
	resp, err := retryDo(context.Background(), fastPolicy(RetryClassRecognition), func() (*httpResponse, error) {
		calls++
		if calls < 2 {
			return &httpResponse{HTTPStatus: 502}, nil
		}
		return &httpResponse{HTTPStatus: 200}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.HTTPStatus)
	assert.Equal(t, 2, calls)
}

func TestRetry_Recognition_DoesNotRetry429(t *testing.T) {
	calls := 0
	resp, err := retryDo(context.Background(), fastPolicy(RetryClassRecognition), func() (*httpResponse, error) {
		calls++
		return &httpResponse{HTTPStatus: 429}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 429, resp.HTTPStatus)
	assert.Equal(t, 1, calls)
}

func TestRetry_Recognition_RetriesOnPreUploadError(t *testing.T) {
	preUpload := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	calls := 0
	_, err := retryDo(context.Background(), fastPolicy(RetryClassRecognition), func() (*httpResponse, error) {
		calls++
		if calls < 2 {
			return nil, preUpload
		}
		return &httpResponse{HTTPStatus: 200}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestRetry_Recognition_DoesNotRetryPostUploadError(t *testing.T) {
	postUpload := &net.OpError{Op: "read", Err: errors.New("read timeout")}
	calls := 0
	_, err := retryDo(context.Background(), fastPolicy(RetryClassRecognition), func() (*httpResponse, error) {
		calls++
		return nil, postUpload
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "recognition must not retry post-upload errors (cost protection)")
}

func TestRetry_Read_RetriesAnyNetError(t *testing.T) {
	netErr := &net.OpError{Op: "read", Err: errors.New("read timeout")}
	calls := 0
	_, err := retryDo(context.Background(), fastPolicy(RetryClassRead), func() (*httpResponse, error) {
		calls++
		if calls < 2 {
			return nil, netErr
		}
		return &httpResponse{HTTPStatus: 200}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestRetry_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	policy := RetryPolicy{Class: RetryClassRead, MaxAttempts: 3, BackoffFactor: 50 * time.Millisecond, BackoffMax: time.Second}
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	calls := 0
	_, err := retryDo(ctx, policy, func() (*httpResponse, error) {
		calls++
		return &httpResponse{HTTPStatus: 503}, nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestIsPreUploadConnectionError(t *testing.T) {
	assert.True(t, isPreUploadConnectionError(&net.OpError{Op: "dial", Err: errors.New("x")}))
	assert.True(t, isPreUploadConnectionError(&net.DNSError{Err: "no such host"}))
	assert.False(t, isPreUploadConnectionError(&net.OpError{Op: "read", Err: errors.New("x")}))
	assert.False(t, isPreUploadConnectionError(nil))
	assert.False(t, isPreUploadConnectionError(errors.New("plain")))
}

func TestRetryPolicy_DefaultsAreSane(t *testing.T) {
	d := defaultRetryPolicy(RetryClassRead)
	assert.Equal(t, 3, d.MaxAttempts)
	assert.Equal(t, 500*time.Millisecond, d.BackoffFactor)
	assert.Equal(t, 30*time.Second, d.BackoffMax)
}

func TestRetryPolicy_WithDefaults_FillsZeroFields(t *testing.T) {
	p := RetryPolicy{Class: RetryClassRead, MaxAttempts: 5}.withDefaults()
	assert.Equal(t, 5, p.MaxAttempts)
	assert.Equal(t, 500*time.Millisecond, p.BackoffFactor)
}

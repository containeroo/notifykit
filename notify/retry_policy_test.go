package notify

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type retryNetworkError struct {
	timeout bool
}

func (e retryNetworkError) Error() string   { return "network failed" }
func (e retryNetworkError) Timeout() bool   { return e.timeout }
func (e retryNetworkError) Temporary() bool { return true }

func TestPermanent(t *testing.T) {
	t.Parallel()

	t.Run("preserves nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, Permanent(nil))
	})

	t.Run("marks wrapped error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		err := Permanent(boom)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.True(t, IsPermanent(err))
	})

	t.Run("preserves existing permanent error", func(t *testing.T) {
		t.Parallel()

		err := Permanent(errors.New("boom"))
		assert.Equal(t, err, Permanent(err))
	})
}

func TestRetryOnError(t *testing.T) {
	t.Parallel()

	t.Run("retries ordinary error", func(t *testing.T) {
		t.Parallel()

		assert.True(t, RetryOnError(DeliveryResult{}, errors.New("boom")))
	})

	t.Run("does not retry permanent error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, RetryOnError(DeliveryResult{}, Permanent(errors.New("boom"))))
	})

	t.Run("does not retry success", func(t *testing.T) {
		t.Parallel()

		assert.False(t, RetryOnError(DeliveryResult{}, nil))
	})
}

func TestRetryOnStatusCode(t *testing.T) {
	t.Parallel()

	policy := RetryOnStatusCode(408, 429)

	t.Run("retries configured status", func(t *testing.T) {
		t.Parallel()

		assert.True(t, policy(DeliveryResult{StatusCode: 429}, errors.New("too many requests")))
	})

	t.Run("does not retry other status", func(t *testing.T) {
		t.Parallel()

		assert.False(t, policy(DeliveryResult{StatusCode: 400}, errors.New("bad request")))
	})

	t.Run("does not retry permanent error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, policy(DeliveryResult{StatusCode: 429}, Permanent(errors.New("bad config"))))
	})
}

func TestRetryOnServerError(t *testing.T) {
	t.Parallel()

	t.Run("retries 500", func(t *testing.T) {
		t.Parallel()

		assert.True(t, RetryOnServerError(DeliveryResult{StatusCode: 500}, errors.New("server error")))
	})

	t.Run("retries 599", func(t *testing.T) {
		t.Parallel()

		assert.True(t, RetryOnServerError(DeliveryResult{StatusCode: 599}, errors.New("server error")))
	})

	t.Run("does not retry 499", func(t *testing.T) {
		t.Parallel()

		assert.False(t, RetryOnServerError(DeliveryResult{StatusCode: 499}, errors.New("client error")))
	})

	t.Run("does not retry permanent server error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, RetryOnServerError(DeliveryResult{StatusCode: 503}, Permanent(errors.New("bad config"))))
	})
}

func TestRetryOnTimeout(t *testing.T) {
	t.Parallel()

	t.Run("retries context deadline", func(t *testing.T) {
		t.Parallel()

		assert.True(t, RetryOnTimeout(DeliveryResult{}, context.DeadlineExceeded))
	})

	t.Run("retries network timeout", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "Post", URL: "https://example.test", Err: retryNetworkError{timeout: true}}
		assert.True(t, RetryOnTimeout(DeliveryResult{}, err))
	})

	t.Run("does not retry non-timeout network error", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "Post", URL: "https://example.test", Err: retryNetworkError{}}
		assert.False(t, RetryOnTimeout(DeliveryResult{}, err))
	})
}

func TestRetryOnNetworkError(t *testing.T) {
	t.Parallel()

	t.Run("retries transport error", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "Post", URL: "https://example.test", Err: retryNetworkError{}}
		assert.True(t, RetryOnNetworkError(DeliveryResult{}, err))
	})

	t.Run("does not retry timeout", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "Post", URL: "https://example.test", Err: retryNetworkError{timeout: true}}
		assert.False(t, RetryOnNetworkError(DeliveryResult{}, err))
	})

	t.Run("does not mistake malformed url for network failure", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "parse", URL: "://bad-url", Err: errors.New("missing protocol scheme")}
		assert.False(t, RetryOnNetworkError(DeliveryResult{}, err))
	})

	t.Run("does not retry permanent network error", func(t *testing.T) {
		t.Parallel()

		err := Permanent(retryNetworkError{})
		assert.False(t, RetryOnNetworkError(DeliveryResult{}, err))
	})
}

func TestAnyRetryPolicy(t *testing.T) {
	t.Parallel()

	policy := AnyRetryPolicy(
		nil,
		RetryOnStatusCode(409),
		RetryOnServerError,
	)

	t.Run("retries when one policy matches", func(t *testing.T) {
		t.Parallel()

		assert.True(t, policy(DeliveryResult{StatusCode: 409}, errors.New("conflict")))
	})

	t.Run("retries when later policy matches", func(t *testing.T) {
		t.Parallel()

		assert.True(t, policy(DeliveryResult{StatusCode: 503}, errors.New("unavailable")))
	})

	t.Run("does not retry when no policy matches", func(t *testing.T) {
		t.Parallel()

		assert.False(t, policy(DeliveryResult{StatusCode: 400}, errors.New("bad request")))
	})
}

func TestDefaultRetryPolicy(t *testing.T) {
	t.Parallel()

	t.Run("retries request timeout status", func(t *testing.T) {
		t.Parallel()

		assert.True(t, DefaultRetryPolicy(DeliveryResult{StatusCode: 408}, errors.New("request timeout")))
	})

	t.Run("retries rate limit status", func(t *testing.T) {
		t.Parallel()

		assert.True(t, DefaultRetryPolicy(DeliveryResult{StatusCode: 429}, errors.New("rate limited")))
	})

	t.Run("retries server error", func(t *testing.T) {
		t.Parallel()

		assert.True(t, DefaultRetryPolicy(DeliveryResult{StatusCode: 502}, errors.New("bad gateway")))
	})

	t.Run("retries timeout", func(t *testing.T) {
		t.Parallel()

		assert.True(t, DefaultRetryPolicy(DeliveryResult{}, context.DeadlineExceeded))
	})

	t.Run("retries network error", func(t *testing.T) {
		t.Parallel()

		err := &url.Error{Op: "Post", URL: "https://example.test", Err: retryNetworkError{}}
		assert.True(t, DefaultRetryPolicy(DeliveryResult{}, err))
	})

	t.Run("does not retry client error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, DefaultRetryPolicy(DeliveryResult{StatusCode: 400}, errors.New("bad request")))
	})

	t.Run("does not retry generic error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, DefaultRetryPolicy(DeliveryResult{}, errors.New("boom")))
	})

	t.Run("does not retry permanent error", func(t *testing.T) {
		t.Parallel()

		assert.False(t, DefaultRetryPolicy(DeliveryResult{StatusCode: 503}, Permanent(errors.New("bad config"))))
	})
}

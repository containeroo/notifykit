package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithRetry tests expected behavior.
func TestWithRetry(t *testing.T) {
	t.Parallel()

	t.Run("returns success on first attempt", func(t *testing.T) {
		t.Parallel()

		result, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{Count: 3}, func() (DeliveryResult, error) {
			return DeliveryResult{Status: "ok"}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Status)
		assert.Equal(t, 1, attempts)
	})

	t.Run("retries until success", func(t *testing.T) {
		t.Parallel()

		calls := 0
		_, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{Count: 2, Policy: RetryOnError}, func() (DeliveryResult, error) {
			calls++
			if calls < 2 {
				return DeliveryResult{}, errors.New("not yet")
			}
			return DeliveryResult{}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, 2, calls)
	})

	t.Run("does not retry without policy", func(t *testing.T) {
		t.Parallel()

		calls := 0
		boom := errors.New("boom")
		_, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{Count: 2}, func() (DeliveryResult, error) {
			calls++
			return DeliveryResult{StatusCode: 503}, boom
		})

		require.ErrorIs(t, err, boom)
		assert.Equal(t, 1, attempts)
		assert.Equal(t, 1, calls)
	})

	t.Run("uses default retry policy when configured", func(t *testing.T) {
		t.Parallel()

		calls := 0
		_, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{Count: 2, Policy: DefaultRetryPolicy}, func() (DeliveryResult, error) {
			calls++
			if calls == 1 {
				return DeliveryResult{StatusCode: 503}, errors.New("unavailable")
			}
			return DeliveryResult{StatusCode: 200}, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, 2, calls)
	})

	t.Run("uses custom retry policy", func(t *testing.T) {
		t.Parallel()

		calls := 0
		_, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{
			Count:  2,
			Policy: RetryOnStatusCode(409),
		}, func() (DeliveryResult, error) {
			calls++
			if calls == 1 {
				return DeliveryResult{StatusCode: 409}, errors.New("conflict")
			}
			return DeliveryResult{StatusCode: 200}, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, 2, calls)
	})
}

// TestWithRetryInternal tests expected behavior.
func TestWithRetryInternal(t *testing.T) {
	t.Parallel()

	t.Run("nil context errors", func(t *testing.T) {
		t.Parallel()

		_, attempts, err := withRetry(nil, testLogger(), RetryConfig{}, func() (DeliveryResult, error) { return DeliveryResult{}, nil })
		require.Error(t, err)
		assert.Equal(t, 0, attempts)
	})

	t.Run("nil function errors", func(t *testing.T) {
		t.Parallel()

		_, attempts, err := withRetry(context.Background(), testLogger(), RetryConfig{}, nil)
		require.Error(t, err)
		assert.Equal(t, 0, attempts)
	})

	t.Run("returns last error after attempts", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		calls := 0
		_, attempts, err := withRetry(context.Background(), nil, RetryConfig{Count: 2, Policy: RetryOnError}, func() (DeliveryResult, error) {
			calls++
			return DeliveryResult{}, boom
		})
		require.ErrorIs(t, err, boom)
		assert.Equal(t, 3, attempts)
		assert.Equal(t, 3, calls)
	})

	t.Run("honors canceled context during backoff", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, attempts, err := withRetry(ctx, testLogger(), RetryConfig{Count: 1, Backoff: time.Hour, Policy: RetryOnError}, func() (DeliveryResult, error) {
			calls++
			cancel()
			return DeliveryResult{}, errors.New("boom")
		})
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, attempts)
		assert.Equal(t, 1, calls)
	})
}

// TestRetryDelay tests expected behavior.
func TestRetryDelay(t *testing.T) {
	t.Parallel()

	t.Run("uses local backoff when longer", func(t *testing.T) {
		t.Parallel()

		wait := retryDelay(
			RetryConfig{Backoff: 2 * time.Second},
			1,
			DeliveryResult{RetryAfter: time.Second},
		)

		assert.Equal(t, 2*time.Second, wait)
	})

	t.Run("uses target retry after when longer", func(t *testing.T) {
		t.Parallel()

		wait := retryDelay(
			RetryConfig{Backoff: time.Second, MaxBackoff: 2 * time.Second},
			4,
			DeliveryResult{RetryAfter: 30 * time.Second},
		)

		assert.Equal(t, 30*time.Second, wait)
	})

	t.Run("uses retry after without local backoff", func(t *testing.T) {
		t.Parallel()

		wait := retryDelay(RetryConfig{}, 1, DeliveryResult{RetryAfter: 3 * time.Second})

		assert.Equal(t, 3*time.Second, wait)
	})
}

// TestRetryBackoff tests expected behavior.
func TestRetryBackoff(t *testing.T) {
	t.Parallel()

	t.Run("returns zero without backoff", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, time.Duration(0), retryBackoff(RetryConfig{Backoff: 0}, 1))
	})

	t.Run("returns exponential backoff", func(t *testing.T) {
		t.Parallel()

		cfg := RetryConfig{Backoff: time.Second}

		assert.Equal(t, time.Second, retryBackoff(cfg, 1))
		assert.Equal(t, 2*time.Second, retryBackoff(cfg, 2))
		assert.Equal(t, 4*time.Second, retryBackoff(cfg, 3))
		assert.Equal(t, 8*time.Second, retryBackoff(cfg, 4))
	})

	t.Run("caps at max backoff", func(t *testing.T) {
		t.Parallel()

		cfg := RetryConfig{
			Backoff:    time.Second,
			MaxBackoff: 3 * time.Second,
		}

		assert.Equal(t, time.Second, retryBackoff(cfg, 1))
		assert.Equal(t, 2*time.Second, retryBackoff(cfg, 2))
		assert.Equal(t, 3*time.Second, retryBackoff(cfg, 3))
		assert.Equal(t, 3*time.Second, retryBackoff(cfg, 4))
	})

	t.Run("applies full jitter", func(t *testing.T) {
		t.Parallel()

		wait := retryBackoff(RetryConfig{Backoff: time.Second, Jitter: true}, 3)

		assert.GreaterOrEqual(t, wait, time.Duration(0))
		assert.Less(t, wait, 4*time.Second)
	})

	t.Run("jitters capped backoff", func(t *testing.T) {
		t.Parallel()

		wait := retryBackoff(RetryConfig{
			Backoff:    time.Second,
			MaxBackoff: 3 * time.Second,
			Jitter:     true,
		}, 4)

		assert.GreaterOrEqual(t, wait, time.Duration(0))
		assert.Less(t, wait, 3*time.Second)
	})
}

// TestJitterBackoff tests expected behavior.
func TestJitterBackoff(t *testing.T) {
	t.Parallel()

	t.Run("returns zero for zero duration", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, time.Duration(0), jitterBackoff(0))
	})

	t.Run("returns delay below upper bound", func(t *testing.T) {
		t.Parallel()

		wait := jitterBackoff(time.Second)

		assert.GreaterOrEqual(t, wait, time.Duration(0))
		assert.Less(t, wait, time.Second)
	})
}

// TestDoubleDuration tests expected behavior.
func TestDoubleDuration(t *testing.T) {
	t.Parallel()

	t.Run("doubles duration", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 2*time.Second, doubleDuration(time.Second))
	})

	t.Run("saturates on overflow", func(t *testing.T) {
		t.Parallel()

		const maxDuration time.Duration = 1<<63 - 1

		assert.Equal(t, maxDuration, doubleDuration(maxDuration))
	})
}

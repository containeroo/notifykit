package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"time"
)

// withRetry executes fn until it succeeds, the context ends, or attempts run out.
func withRetry(
	ctx context.Context,
	logger *slog.Logger,
	cfg RetryConfig,
	fn func() (DeliveryResult, error),
) (DeliveryResult, int, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if ctx == nil {
		return DeliveryResult{}, 0, errors.New("context is nil")
	}
	if fn == nil {
		return DeliveryResult{}, 0, errors.New("retry function is nil")
	}

	maxAttempts := max(cfg.Count+1, 1)
	policy := cfg.Policy
	if policy == nil {
		policy = DefaultRetryPolicy
	}
	var (
		lastResult DeliveryResult
		lastErr    error
		executed   int
	)

	for attempt := range maxAttempts {
		if attempt > 0 {
			wait := retryBackoff(cfg, attempt)
			if wait > 0 {
				logger.Debug(
					"notification target retry",
					"attempt",
					attempt+1,
					"backoff",
					wait.String(),
				)

				if err := waitForRetry(ctx, wait); err != nil {
					return lastResult, executed, err
				}
			}
		}

		select {
		case <-ctx.Done():
			return lastResult, executed, ctx.Err()
		default:
		}

		result, err := fn()
		executed++
		lastResult = result
		if err == nil {
			return result, executed, nil
		}

		lastErr = err
		if ctx.Err() != nil {
			return lastResult, executed, ctx.Err()
		}
		if executed >= maxAttempts || !policy(result, err) {
			return lastResult, executed, lastErr
		}
	}

	return lastResult, executed, lastErr
}

// waitForRetry waits for duration or returns when ctx is canceled.
func waitForRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryBackoff returns the wait duration before retry attempt.
//
// retry is one-based: retry 1 is the first retry after the initial attempt.
func retryBackoff(cfg RetryConfig, retry int) time.Duration {
	if cfg.Backoff <= 0 || retry <= 0 {
		return 0
	}

	wait := cfg.Backoff
	for range retry - 1 {
		wait = doubleDuration(wait)
		if cfg.MaxBackoff > 0 && wait >= cfg.MaxBackoff {
			wait = cfg.MaxBackoff
			break
		}
	}

	if cfg.MaxBackoff > 0 && wait > cfg.MaxBackoff {
		wait = cfg.MaxBackoff
	}
	if cfg.Jitter {
		return jitterBackoff(wait)
	}
	return wait
}

// jitterBackoff returns a full-jitter delay from zero up to duration.
func jitterBackoff(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(duration)))
}

// doubleDuration doubles duration and saturates at the largest duration value.
func doubleDuration(duration time.Duration) time.Duration {
	const maxDuration time.Duration = 1<<63 - 1

	if duration > maxDuration/2 {
		return maxDuration
	}
	return duration * 2
}

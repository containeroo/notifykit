package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

// WithRetry executes fn with retry attempts and exponential backoff.
func WithRetry(
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
	return withRetry(ctx, logger, cfg, fn)
}

// withRetry executes fn until it succeeds, the context ends, or attempts run out.
func withRetry(
	ctx context.Context,
	logger *slog.Logger,
	cfg RetryConfig,
	fn func() (DeliveryResult, error),
) (DeliveryResult, int, error) {
	maxAttempts := max(cfg.Count+1, 1)
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
		if err != nil {
			lastErr = err
			continue
		}
		return result, executed, nil
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
			return cfg.MaxBackoff
		}
	}

	if cfg.MaxBackoff > 0 && wait > cfg.MaxBackoff {
		return cfg.MaxBackoff
	}
	return wait
}

// doubleDuration doubles duration and saturates at the largest duration value.
func doubleDuration(duration time.Duration) time.Duration {
	const maxDuration time.Duration = 1<<63 - 1

	if duration > maxDuration/2 {
		return maxDuration
	}
	return duration * 2
}

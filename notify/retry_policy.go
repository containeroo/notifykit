package notify

import (
	"context"
	"errors"
	"net"
	"net/url"
)

// permanentError marks an error as non-retryable for built-in retry policies.
type permanentError struct {
	err error
}

// Error returns the wrapped error message.
func (e permanentError) Error() string { return e.err.Error() }

// Unwrap returns the wrapped error.
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as non-retryable for Notifykit's built-in retry policies.
//
// Custom policies remain free to inspect or retry the wrapped error.
func Permanent(err error) error {
	if err == nil || IsPermanent(err) {
		return err
	}
	return permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var permanent permanentError
	return errors.As(err, &permanent)
}

// RetryOnError retries every non-permanent error.
//
// This matches Notifykit's original retry behavior and is useful when a target
// has no richer result or error classification.
func RetryOnError(_ DeliveryResult, err error) bool {
	return err != nil && !IsPermanent(err)
}

// RetryOnStatusCode returns a policy that retries the listed status codes.
func RetryOnStatusCode(codes ...int) RetryPolicy {
	allowed := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		allowed[code] = struct{}{}
	}

	return func(result DeliveryResult, err error) bool {
		if err == nil || IsPermanent(err) {
			return false
		}
		_, ok := allowed[result.StatusCode]
		return ok
	}
}

// RetryOnServerError retries status codes from 500 through 599.
func RetryOnServerError(result DeliveryResult, err error) bool {
	if err == nil || IsPermanent(err) {
		return false
	}
	return result.StatusCode >= 500 && result.StatusCode <= 599
}

// RetryOnTimeout retries context deadlines and network timeout errors.
func RetryOnTimeout(_ DeliveryResult, err error) bool {
	if err == nil || IsPermanent(err) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var networkErr net.Error
	return errors.As(transportError(err), &networkErr) && networkErr.Timeout()
}

// RetryOnNetworkError retries non-timeout network transport errors.
func RetryOnNetworkError(_ DeliveryResult, err error) bool {
	if err == nil || IsPermanent(err) {
		return false
	}

	var networkErr net.Error
	return errors.As(transportError(err), &networkErr) && !networkErr.Timeout()
}

// AnyRetryPolicy combines policies and retries when any policy accepts the failure.
func AnyRetryPolicy(policies ...RetryPolicy) RetryPolicy {
	return func(result DeliveryResult, err error) bool {
		for _, policy := range policies {
			if policy != nil && policy(result, err) {
				return true
			}
		}
		return false
	}
}

var defaultRetryPolicy = AnyRetryPolicy(
	RetryOnTimeout,
	RetryOnNetworkError,
	RetryOnStatusCode(408),
	RetryOnStatusCode(429),
	RetryOnServerError,
)

// DefaultRetryPolicy applies Notifykit's conservative transient-failure policy.
//
// It retries timeouts, network transport failures, HTTP-style 408 and 429
// responses, and 5xx responses. Other failures are not retried by default.
func DefaultRetryPolicy(result DeliveryResult, err error) bool {
	return defaultRetryPolicy(result, err)
}

// transportError unwraps URL request errors so malformed URLs are not mistaken
// for network failures merely because url.Error implements net.Error itself.
func transportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

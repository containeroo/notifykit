package notify

import (
	"context"
	"errors"
	"net"
	"slices"
)

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

// RetryOnTimeout retries transport failures that represent a timeout.
func RetryOnTimeout(_ DeliveryResult, err error) bool {
	if err == nil || !IsTransport(err) {
		return false
	}
	return isTimeoutError(err)
}

// RetryOnNetworkError retries non-timeout transport failures.
func RetryOnNetworkError(_ DeliveryResult, err error) bool {
	if err == nil || !IsTransport(err) {
		return false
	}
	return !isTimeoutError(err)
}

// AnyRetryPolicy combines policies and retries when any policy accepts the failure.
func AnyRetryPolicy(policies ...RetryPolicy) RetryPolicy {
	policies = slices.Clone(policies)
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
	RetryOnStatusCode(408, 429),
	RetryOnServerError,
)

// DefaultRetryPolicy applies Notifykit's conservative transient-failure policy.
//
// It retries classified transport failures, HTTP-style 408 and 429 responses,
// and 5xx responses. Other failures are not retried by default.
func DefaultRetryPolicy(result DeliveryResult, err error) bool {
	return defaultRetryPolicy(result, err)
}

// isTimeoutError reports whether a transport error represents a timeout.
func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout()
}

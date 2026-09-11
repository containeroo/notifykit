package notify

import (
	"errors"
	"fmt"
)

// permanentError marks an error as non-retryable for built-in retry policies.
type permanentError struct {
	err error
}

// Error returns the wrapped error message.
func (e permanentError) Error() string { return e.err.Error() }

// Unwrap returns the wrapped error.
func (e permanentError) Unwrap() error { return e.err }

// transportError marks an error as a transport failure.
type transportError struct {
	err error
}

// Error returns the wrapped error message.
func (e transportError) Error() string { return e.err.Error() }

// Unwrap returns the wrapped error.
func (e transportError) Unwrap() error { return e.err }

// Permanent marks err as non-retryable for Notifykit's built-in retry policies.
//
// Permanent classification takes precedence over transport classification.
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

// Transport marks err as a transport failure for Notifykit's built-in retry policies.
//
// Built-in targets use this for network and response-stream failures. Permanent
// errors are never reclassified as transport failures.
func Transport(err error) error {
	if err == nil || IsPermanent(err) || IsTransport(err) {
		return err
	}
	return transportError{err: err}
}

// IsTransport reports whether err was marked with Transport.
//
// Permanent classification takes precedence when an error chain contains both.
func IsTransport(err error) bool {
	if err == nil || IsPermanent(err) {
		return false
	}
	var transport transportError
	return errors.As(err, &transport)
}

// DeliveryError identifies a failed target within its receiver. TargetIndex is zero-based.
// Result may contain sensitive response data and is never included in Error().
type DeliveryError struct {
	ReceiverID  ReceiverID
	TargetType  string
	TargetIndex int
	Attempts    int
	Result      DeliveryResult
	Err         error
}

func (e *DeliveryError) Error() string {
	return fmt.Sprintf("receiver %q target %s[%d] failed after %d attempts: %v", e.ReceiverID, e.TargetType, e.TargetIndex, e.Attempts, e.Err)
}
func (e *DeliveryError) Unwrap() error { return e.Err }

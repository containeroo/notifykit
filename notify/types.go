package notify

import (
	"context"
	"time"
)

// ReceiverID identifies a receiver in a Receivers map.
//
// Notification routing uses receiver IDs, not Receiver.Name. Receiver.Name is a
// display/runtime payload value, while ReceiverID is the stable map key used by
// ReceiverIDs and receiver lookup.
type ReceiverID string

// Receivers maps receiver IDs to receiver configuration.
type Receivers map[ReceiverID]*Receiver

// Notification describes one notification and its template data.
//
// A notification may optionally implement ReceiverRouter to select specific
// receivers. Notifications that do not implement ReceiverRouter are sent to all
// configured receivers.
type Notification interface {
	ID() string
	Data(receiver string, vars map[string]any, subject string) any
}

// ReceiverRouter optionally routes a notification to receiver IDs.
//
// Returning nil or an empty slice sends to all configured receivers.
type ReceiverRouter interface {
	ReceiverIDs() []ReceiverID
}

// Delivery sends a receiver-scoped payload to receivers.
type Delivery interface {
	Dispatch(ctx context.Context, payload Payload, receivers []*Receiver) error
}

// Target delivers a notification payload to one destination.
type Target interface {
	Send(ctx context.Context, payload Payload) (DeliveryResult, error)
	Type() string
}

// DeliveryResult captures target response details for logs and errors.
type DeliveryResult struct {
	// Status is a target-specific delivery status such as sent or failed.
	Status string

	// StatusCode is the target response status code when one exists.
	StatusCode int

	// Response is a short target response summary suitable for logs.
	Response string
}

// ResultTarget returns target-specific delivery details.
type ResultTarget interface {
	SendResult(ctx context.Context, payload Payload) (DeliveryResult, error)
}

// Notifier enqueues notifications for delivery.
type Notifier interface {
	Enqueue(ctx context.Context, notification Notification) (string, error)
}

// Payload is the receiver-scoped notification payload given to targets.
type Payload struct {
	// Notification is the original application notification.
	Notification Notification

	// Receiver is the receiver display name passed to template data.
	Receiver string

	// Vars contains receiver-scoped template variables.
	Vars map[string]any
}

// Data returns notification template data for this receiver and subject.
func (p Payload) Data(subject string) any {
	if p.Notification == nil {
		return nil
	}
	return p.Notification.Data(p.Receiver, p.Vars, subject)
}

// ID returns the notification ID.
func (p Payload) ID() string {
	if p.Notification == nil {
		return ""
	}
	return p.Notification.ID()
}

// Receiver describes runtime delivery configuration.
type Receiver struct {
	// ID is the receiver map key used for routing.
	//
	// NewManager and Send fill this from the Receivers map key when it is unset.
	ID ReceiverID

	// Name is the display name passed to notification template data.
	//
	// NewManager and Send default Name to the receiver ID when it is unset.
	Name string

	// Retry controls retry behavior for all targets on this receiver.
	Retry RetryConfig

	// Targets contains the delivery targets for this receiver.
	Targets []Target

	// Vars contains receiver-scoped template variables.
	Vars map[string]any
}

// RetryConfig defines retry behavior for a receiver.
type RetryConfig struct {
	// Count is the number of retries after the initial attempt.
	//
	// For example, Count 2 means up to 3 total attempts.
	Count int

	// Backoff is the initial wait duration before retrying.
	//
	// Each later retry wait doubles this duration until MaxBackoff caps it.
	// If Backoff is zero or negative, retries happen immediately.
	Backoff time.Duration

	// MaxBackoff caps retry wait durations.
	//
	// If MaxBackoff is zero or negative, retry waits are not capped.
	MaxBackoff time.Duration
}

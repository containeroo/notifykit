package notify

import "context"

// NewReceiver constructs a receiver with id and optional targets.
//
// The receiver ID is the routing key used by ReceiverRouter.ReceiverIDs and
// Receivers maps. Name defaults to the ID during delivery when it is not set.
func NewReceiver(id ReceiverID, targets ...Target) *Receiver {
	return &Receiver{
		ID:      id,
		Name:    string(id),
		Targets: targets,
	}
}

// NewReceivers constructs a receiver map from receiver values.
//
// Nil receivers are ignored. If duplicate IDs are provided, the later receiver
// wins. Receivers with an empty ID are included under the empty key and will
// only be selected when all receivers are used or an empty ID is requested.
func NewReceivers(receivers ...*Receiver) Receivers {
	out := make(Receivers, len(receivers))
	for _, receiver := range receivers {
		if receiver == nil {
			continue
		}
		out[receiver.ID] = receiver
	}
	return normalizeReceivers(out)
}

// SendTo synchronously delivers notification to receivers without requiring a map.
//
// SendTo is a convenience wrapper around Send for simple usage. It builds a
// Receivers map from receivers, uses a discard logger, resolves receiver routing
// the same way as Send, and returns after delivery completes.
func SendTo(ctx context.Context, notification Notification, receivers ...*Receiver) error {
	return Send(ctx, notification, NewReceivers(receivers...), nil)
}

// WithName sets the receiver display name and returns r.
//
// The display name is passed to notification template data as the receiver name.
// It is not used as the routing ID.
func (r *Receiver) WithName(name string) *Receiver {
	if r == nil {
		return nil
	}
	r.Name = name
	return r
}

// WithRetry sets the receiver retry configuration and returns r.
func (r *Receiver) WithRetry(cfg RetryConfig) *Receiver {
	if r == nil {
		return nil
	}
	r.Retry = cfg
	return r
}

// WithTargets appends targets to the receiver and returns r.
func (r *Receiver) WithTargets(targets ...Target) *Receiver {
	if r == nil {
		return nil
	}
	r.Targets = append(r.Targets, targets...)
	return r
}

// WithCustomData sets receiver-scoped custom template data and returns r.
func (r *Receiver) WithCustomData(customData map[string]any) *Receiver {
	if r == nil {
		return nil
	}
	r.CustomData = customData
	return r
}

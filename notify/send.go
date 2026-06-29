package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// Send synchronously delivers notification to the configured receivers.
//
// Send is a convenience helper for simple use cases that do not need Manager's
// in-memory queue or asynchronous dispatcher. It resolves receivers using the
// same routing rule as Manager: ReceiverRouter.ReceiverIDs returns receiver map
// keys, and nil or empty receiver IDs send to all configured receivers.
//
// If logger is nil, a discard logger is used. Send returns an error when the
// context, notification, delivery, or resolved receiver set is invalid, or when
// one or more targets fail.
func Send(ctx context.Context, notification Notification, receivers Receivers, logger *slog.Logger) error {
	if ctx == nil {
		return errors.New("context is nil")
	}
	if notification == nil {
		return errors.New("notification is nil")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	receivers = normalizeReceivers(receivers)
	resolved := resolveReceivers(receivers, receiverIDs(notification), logger)
	if len(resolved) == 0 {
		return errors.New("no receivers resolved")
	}

	return NewDelivery(logger).Dispatch(ctx, Payload{Notification: notification}, resolved)
}

// normalizeReceivers applies receiver ID and display-name defaults.
func normalizeReceivers(receivers Receivers) Receivers {
	if receivers == nil {
		return Receivers{}
	}
	out := make(Receivers, len(receivers))
	for id, receiver := range receivers {
		if receiver == nil {
			out[id] = nil
			continue
		}
		normalized := *receiver
		if receiver.ID == "" {
			normalized.ID = id
		}
		if receiver.Name == "" {
			normalized.Name = string(id)
		}
		out[id] = &normalized
	}
	return out
}

// receiverIDs returns the receiver IDs requested by notification.
func receiverIDs(notification Notification) []ReceiverID {
	if notification == nil {
		return nil
	}
	if routed, ok := notification.(ReceiverRouter); ok {
		return routed.ReceiverIDs()
	}
	return nil
}

// resolveReceivers returns all named receivers or every receiver when no IDs are set.
func resolveReceivers(receivers Receivers, ids []ReceiverID, logger *slog.Logger) []*Receiver {
	if len(ids) == 0 {
		out := make([]*Receiver, 0, len(receivers))
		for _, receiver := range receivers {
			if receiver == nil {
				continue
			}
			out = append(out, receiver)
		}
		return out
	}

	out := make([]*Receiver, 0, len(ids))
	for _, id := range ids {
		receiver, ok := receivers[id]
		if !ok {
			logger.Warn("receiver not found", "receiverID", id)
			continue
		}
		if receiver == nil {
			logger.Warn("receiver is nil", "receiverID", id)
			continue
		}
		out = append(out, receiver)
	}
	return out
}

package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// Dispatcher dequeues notifications and delivers them.
type Dispatcher struct {
	store     *Store
	mailbox   <-chan string
	delivery  Delivery
	receivers Receivers
	logger    *slog.Logger
}

// NewDispatcher constructs a dispatcher for an existing store, mailbox,
// delivery engine, and receiver map.
//
// Most applications should use NewManager for queued asynchronous delivery or
// Send for simple synchronous delivery. NewDispatcher is useful when callers
// need to provide their own store, mailbox, or delivery implementation.
func NewDispatcher(
	store *Store,
	mailbox <-chan string,
	delivery Delivery,
	receivers Receivers,
	logger *slog.Logger,
) (*Dispatcher, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if mailbox == nil {
		return nil, errors.New("mailbox is required")
	}
	if delivery == nil {
		return nil, errors.New("delivery is required")
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	receivers = normalizeReceivers(receivers)

	return &Dispatcher{
		store:     store,
		mailbox:   mailbox,
		delivery:  delivery,
		receivers: receivers,
		logger:    logger,
	}, nil
}

// Start processes queued notifications until ctx is canceled or the mailbox closes.
func (d *Dispatcher) Start(ctx context.Context) {
	if d == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-d.mailbox:
			if !ok {
				return
			}
			d.dispatch(ctx, id)
		}
	}
}

// dispatch delivers one queued notification by queue id.
func (d *Dispatcher) dispatch(ctx context.Context, queueID string) {
	n, ok := d.store.Get(queueID)
	if !ok {
		d.logger.Warn("notification not found", "queueID", queueID)
		return
	}
	d.store.Delete(queueID)

	receivers := d.resolveReceivers(receiverIDs(n))
	if len(receivers) == 0 {
		d.logger.Warn("no receivers resolved", "queueID", queueID, "notificationID", n.ID())
		return
	}

	payload := Payload{Notification: n}
	if err := d.delivery.Dispatch(ctx, payload, receivers); err != nil {
		d.logger.Error("notification delivery failed", "queueID", queueID, "notificationID", n.ID(), "error", err)
		return
	}
	d.logger.Info("notification delivered", "queueID", queueID, "notificationID", n.ID())
}

// resolveReceivers returns all named receivers or every receiver when no IDs are set.
func (d *Dispatcher) resolveReceivers(ids []ReceiverID) []*Receiver {
	return resolveReceivers(d.receivers, ids, d.logger)
}

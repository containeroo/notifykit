package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// dispatcher dequeues notifications and delivers them.
type dispatcher struct {
	store     *store
	mailbox   <-chan string
	delivery  delivery
	receivers Receivers
	logger    *slog.Logger
}

// newDispatcher constructs a dispatcher for an existing store, mailbox,
// delivery engine, and receiver map.
//
// Most applications should use NewManager for queued asynchronous delivery or
// Send for simple synchronous delivery. It is kept internal so Notifykit exposes
// only the higher-level Send and Manager APIs.
func newDispatcher(
	store *store,
	mailbox <-chan string,
	delivery delivery,
	receivers Receivers,
	logger *slog.Logger,
) (*dispatcher, error) {
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

	return &dispatcher{
		store:     store,
		mailbox:   mailbox,
		delivery:  delivery,
		receivers: receivers,
		logger:    logger,
	}, nil
}

// start processes queued notifications until ctx is canceled or the mailbox closes.
func (d *dispatcher) start(ctx context.Context) {
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
func (d *dispatcher) dispatch(ctx context.Context, queueID string) {
	n, ok := d.store.get(queueID)
	if !ok {
		d.logger.Warn("notification not found", "queueID", queueID)
		return
	}
	d.store.delete(queueID)

	receivers := d.resolveReceivers(receiverIDs(n))
	if len(receivers) == 0 {
		d.logger.Warn("no receivers resolved", "queueID", queueID, "notificationID", n.ID())
		return
	}

	payload := Payload{Notification: n}
	if err := d.delivery.dispatch(ctx, payload, receivers); err != nil {
		d.logger.Error("notification delivery failed", "queueID", queueID, "notificationID", n.ID(), "error", err)
		return
	}
	d.logger.Info("notification delivered", "queueID", queueID, "notificationID", n.ID())
}

// resolveReceivers returns all named receivers or every receiver when no IDs are set.
func (d *dispatcher) resolveReceivers(ids []ReceiverID) []*Receiver {
	return resolveReceivers(d.receivers, ids, d.logger)
}

package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"

	"github.com/containeroo/notifykit/ids"
)

// Manager owns notification queueing and dispatch infrastructure.
type Manager struct {
	store      *Store
	mailbox    chan string
	dispatcher *Dispatcher
	receivers  Receivers
}

// NewManager constructs a notification manager for queued asynchronous delivery.
//
// The manager owns an in-memory store, buffered mailbox, dispatcher, and
// delivery engine. Enqueued notifications are routed by ReceiverRouter.ReceiverIDs:
// returned values are matched against the receiver map keys, and nil or empty
// receiver IDs send to all configured receivers.
//
// If receivers is nil, the manager starts with no configured receivers. If
// logger is nil, a discard logger is used.
func NewManager(receivers Receivers, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	receivers = normalizeReceivers(receivers)
	store := NewStore()
	mailbox := make(chan string, 128)
	delivery := NewDelivery(logger)

	dispatcher, err := NewDispatcher(store, mailbox, delivery, receivers, logger)
	if err != nil {
		return nil, err
	}

	return &Manager{
		store:      store,
		mailbox:    mailbox,
		dispatcher: dispatcher,
		receivers:  receivers,
	}, nil
}

// Enqueue stores a notification and queues it for delivery.
func (m *Manager) Enqueue(ctx context.Context, n Notification) (string, error) {
	if m == nil {
		return "", errors.New("manager is nil")
	}
	if ctx == nil {
		return "", errors.New("context is nil")
	}
	if n == nil {
		return "", errors.New("notification is nil")
	}
	if m.mailbox == nil {
		return "", errors.New("mailbox is nil")
	}

	id, err := nextQueueID()
	if err != nil {
		return "", err
	}
	m.store.Put(id, n)

	select {
	case m.mailbox <- id:
		return id, nil
	case <-ctx.Done():
		m.store.Delete(id)
		return "", ctx.Err()
	}
}

// Receivers returns configured receivers sorted by ID.
func (m *Manager) Receivers() []*Receiver {
	if m == nil {
		return nil
	}
	out := make([]*Receiver, 0, len(m.receivers))
	for _, receiver := range m.receivers {
		out = append(out, receiver)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// Start begins asynchronous delivery processing.
func (m *Manager) Start(ctx context.Context) error {
	if m == nil || m.dispatcher == nil {
		return errors.New("manager is nil")
	}
	if ctx == nil {
		return errors.New("context is nil")
	}
	go m.dispatcher.Start(ctx)
	return nil
}

// nextQueueID returns a time-prefixed process-local queue id.
func nextQueueID() (string, error) {
	return ids.NewUUIDV7()
}

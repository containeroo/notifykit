package notify

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/containeroo/uuidv7"
)

var ErrManagerStarted = errors.New("manager already started")

// Manager owns notification queueing and dispatch infrastructure.
type Manager struct {
	store      *store
	mailbox    chan string
	dispatcher *dispatcher
	receivers  Receivers

	workers int

	mu      sync.Mutex
	started bool
}

type ManagerOption func(*managerConfig)

type managerConfig struct {
	workers int
}

// WithWorkers configures how many queued notifications may be processed concurrently.
//
// Values less than 1 are ignored. The default is one worker. Targets used with
// more than one worker must be safe for concurrent calls.
func WithWorkers(workers int) ManagerOption {
	return func(cfg *managerConfig) {
		if workers > 0 {
			cfg.workers = workers
		}
	}
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
func NewManager(receivers Receivers, logger *slog.Logger, opts ...ManagerOption) (*Manager, error) {
	cfg := managerConfig{
		workers: 1,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if cfg.workers <= 0 {
		return nil, errors.New("workers must be greater than zero")
	}

	receivers = normalizeReceivers(receivers)
	store := newStore()
	mailbox := make(chan string, 128)
	delivery := newDelivery(logger)

	dispatcher, err := newDispatcher(store, mailbox, delivery, receivers, logger)
	if err != nil {
		return nil, err
	}

	return &Manager{
		store:      store,
		mailbox:    mailbox,
		dispatcher: dispatcher,
		receivers:  receivers,
		workers:    cfg.workers,
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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if n == nil {
		return "", errors.New("notification is nil")
	}
	if m.store == nil {
		return "", errors.New("store is nil")
	}
	if m.mailbox == nil {
		return "", errors.New("mailbox is nil")
	}

	id, err := nextQueueID()
	if err != nil {
		return "", err
	}

	m.store.put(id, n)

	select {
	case m.mailbox <- id:
		return id, nil
	case <-ctx.Done():
		m.store.delete(id)
		return "", ctx.Err()
	}
}

// Receivers returns configured receivers sorted by ID.
func (m *Manager) Receivers() []*Receiver {
	if m == nil {
		return nil
	}

	receivers := make([]*Receiver, 0, len(m.receivers))
	for _, receiver := range m.receivers {
		if receiver == nil {
			continue
		}
		receivers = append(receivers, receiver)
	}

	slices.SortFunc(receivers, func(a, b *Receiver) int {
		return cmp.Compare(a.ID, b.ID)
	})

	return receivers
}

// Start begins asynchronous delivery processing.
//
// Start launches the configured worker count. Each worker reads from the same
// queue, so different notifications may be delivered concurrently. Start may
// only be called once. Calling Start again returns ErrManagerStarted.
func (m *Manager) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("manager is nil")
	}
	if ctx == nil {
		return errors.New("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.dispatcher == nil {
		return errors.New("dispatcher is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return ErrManagerStarted
	}
	m.started = true

	for range m.workers {
		go m.dispatcher.start(ctx)
	}

	return nil
}

// nextQueueID returns a time-sortable UUIDv7 queue id.
func nextQueueID() (string, error) {
	return uuidv7.New()
}

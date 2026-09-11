package notify

import (
	"cmp"
	"context"
	"errors"
	"io"
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
	slots   chan struct{}

	mu      sync.Mutex
	started bool
}

type ManagerOption func(*managerConfig)

type managerConfig struct {
	workers    int
	capacity   int
	onComplete func(Completion)
}

// Completion reports the final outcome of one queued notification.
// Err can be inspected with errors.As for DeliveryError. Nil means delivery succeeded.
type Completion struct {
	QueueID        string
	NotificationID string
	Err            error
}

// WithOnComplete installs a callback invoked once after each queued delivery.
// It runs synchronously on a worker; it must be concurrency-safe, return promptly,
// and must not wait for manager shutdown or enqueue into the same manager.
func WithOnComplete(fn func(Completion)) ManagerOption {
	return func(c *managerConfig) { c.onComplete = fn }
}

// WithQueueCapacity bounds admitted queued notifications (excluding active deliveries).
// Values below one are rejected by NewManager. The default is 128.
func WithQueueCapacity(capacity int) ManagerOption {
	return func(c *managerConfig) { c.capacity = capacity }
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
		workers:  1,
		capacity: 128,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if cfg.capacity < 1 {
		return nil, errors.New("queue capacity must be greater than zero")
	}
	if cfg.workers <= 0 {
		return nil, errors.New("workers must be greater than zero")
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	receivers = normalizeReceivers(receivers)
	if err := validateReceivers(receivers); err != nil {
		return nil, err
	}

	store := newStore()
	mailbox := make(chan string, cfg.capacity)
	delivery := newDelivery(logger)
	dispatcher := newDispatcher(store, mailbox, delivery, receivers, logger)

	slots := make(chan struct{}, cfg.capacity)
	dispatcher.onDequeue = func() { <-slots }
	dispatcher.onComplete = cfg.onComplete
	return &Manager{
		store:      store,
		mailbox:    mailbox,
		dispatcher: dispatcher,
		receivers:  receivers,
		workers:    cfg.workers,
		slots:      slots,
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
	id, err := nextQueueID()
	if err != nil {
		return "", err
	}

	select {
	case m.slots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	m.store.put(id, n)

	select {
	case m.mailbox <- id:
		return id, nil
	case <-ctx.Done():
		m.store.delete(id)
		<-m.slots
		return "", ctx.Err()
	}
}

// Receivers returns snapshots sorted by ID. Structs, target slices and top-level
// CustomData maps are copied; targets and nested custom values remain shared.
func (m *Manager) Receivers() []*Receiver {
	if m == nil {
		return nil
	}

	receivers := make([]*Receiver, 0, len(m.receivers))
	for _, receiver := range normalizeReceivers(m.receivers) {
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

package notify

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"uuid"
)

var ErrManagerStarted = errors.New("manager already started")

// ErrManagerStopped means the manager no longer accepts notifications.
var ErrManagerStopped = errors.New("manager stopped")

// Manager owns notification queueing and dispatch infrastructure.
type Manager struct {
	store      *store
	mailbox    chan uuid.UUID
	dispatcher *dispatcher
	receivers  Receivers

	workers int
	slots   chan struct{}

	mu          sync.Mutex
	started     bool
	stopped     bool
	finishing   bool
	stopping    chan struct{}
	done        chan struct{}
	runCtx      context.Context
	cancel      context.CancelFunc
	workersDone sync.WaitGroup
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
	QueueID        uuid.UUID
	NotificationID string
	Err            error
}

// WithOnComplete installs a callback invoked once after delivery or discard.
// It runs synchronously on a worker or the shutdown cleanup goroutine; it must be concurrency-safe, return promptly,
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
	mailbox := make(chan uuid.UUID, cfg.capacity)
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
		stopping:   make(chan struct{}),
		done:       make(chan struct{}),
	}, nil
}

// Enqueue stores a notification and queues it for delivery, returning its UUIDv7 queue ID.
func (m *Manager) Enqueue(ctx context.Context, n Notification) (uuid.UUID, error) {
	if m == nil {
		return uuid.UUID{}, errors.New("manager is nil")
	}
	if ctx == nil {
		return uuid.UUID{}, errors.New("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return uuid.UUID{}, err
	}
	if n == nil {
		return uuid.UUID{}, errors.New("notification is nil")
	}

	id := uuid.NewV7()

	select {
	case m.slots <- struct{}{}:
	case <-m.stopping:
		return uuid.UUID{}, ErrManagerStopped
	case <-ctx.Done():
		return uuid.UUID{}, ctx.Err()
	}
	// Admission and mailbox closure share the lock. A reserved slot guarantees
	// that sending below cannot block, even with concurrent producers.
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped || (m.runCtx != nil && m.runCtx.Err() != nil) {
		<-m.slots
		return uuid.UUID{}, ErrManagerStopped
	}
	if err := ctx.Err(); err != nil {
		<-m.slots
		return uuid.UUID{}, err
	}
	m.store.put(id, n)
	m.mailbox <- id
	return id, nil
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

	if m.stopped {
		return ErrManagerStopped
	}
	m.started = true
	m.finishing = true
	m.runCtx, m.cancel = context.WithCancel(ctx)
	context.AfterFunc(m.runCtx, func() { m.stop(false) })
	m.workersDone.Add(m.workers)
	for range m.workers {
		go func() { defer m.workersDone.Done(); m.dispatcher.start(m.runCtx) }()
	}
	go m.finish()

	return nil
}

// Shutdown stops admission and drains accepted notifications. If ctx expires,
// active deliveries are canceled and queued notifications are discarded with
// ErrManagerStopped completion outcomes. Canceling Start's context also aborts
// delivery. Shutdown before Start discards queued work. Managers cannot restart.
// Call Wait to observe completion after a timed-out Shutdown.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil {
		return errors.New("manager is nil")
	}
	if ctx == nil {
		return errors.New("context is nil")
	}
	m.stop(true)
	if err := m.Wait(ctx); err != nil {
		m.stop(false)
		return err
	}
	return nil
}

// Wait waits for all workers and completion callbacks to finish after shutdown
// or cancellation. It does not initiate shutdown. Custom targets must honor ctx.
func (m *Manager) Wait(ctx context.Context) error {
	if m == nil {
		return errors.New("manager is nil")
	}
	if ctx == nil {
		return errors.New("context is nil")
	}
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) stop(drain bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopping)
		close(m.mailbox)
	}
	if !drain && m.cancel != nil {
		m.cancel()
	}
	if !m.finishing {
		m.finishing = true
		go m.finish()
	}
}

func (m *Manager) finish() {
	m.workersDone.Wait()
	m.stop(false)
	// Workers have exited, so any remaining entries were never dispatched.
	for id := range m.mailbox {
		n, ok := m.store.get(id)
		m.store.delete(id)
		<-m.slots
		if ok && m.dispatcher.onComplete != nil {
			m.dispatcher.onComplete(
				Completion{
					QueueID:        id,
					NotificationID: n.ID(),
					Err:            ErrManagerStopped,
				})
		}
	}
	close(m.done)
}

package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestShutdownDrainsAcceptedWork(t *testing.T) {
	completed := make(chan Completion, 3)
	m, err := NewManager(NewReceivers(NewReceiver("ops", &testTarget{})), nil, WithOnComplete(func(c Completion) { completed <- c }))
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		_, err = m.Enqueue(t.Context(), testNotification{id: id})
		require.NoError(t, err)
	}
	require.NoError(t, m.Start(t.Context()))
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, m.Shutdown(ctx))
	require.Len(t, completed, 3)
	for range 3 {
		require.NoError(t, (<-completed).Err)
	}
	_, err = m.Enqueue(t.Context(), testNotification{id: "notification"})
	require.ErrorIs(t, err, ErrManagerStopped)
	require.ErrorIs(t, m.Start(t.Context()), ErrManagerStarted)
	require.NoError(t, m.Shutdown(ctx))
	require.Empty(t, m.store.items)
}

func TestCancelStopsAdmissionAndReleasesProducers(t *testing.T) {
	target := &blockingTarget{entered: make(chan uuid.UUID, 1), release: make(chan struct{})}
	completed := make(chan Completion, 2)
	m, err := NewManager(NewReceivers(NewReceiver("ops", target)), nil, WithQueueCapacity(1), WithOnComplete(func(c Completion) { completed <- c }))
	require.NoError(t, err)
	run, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, m.Start(run))
	active := testNotification{id: "active"}
	_, err = m.Enqueue(t.Context(), active)
	require.NoError(t, err)
	require.Equal(t, active.ID(), receiveWorkerEntry(t, target.entered))
	_, err = m.Enqueue(t.Context(), testNotification{id: "queued"})
	require.NoError(t, err)
	producer := make(chan error, 1)
	go func() { _, err := m.Enqueue(t.Context(), testNotification{id: "blocked"}); producer <- err }()
	cancel()
	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	require.NoError(t, m.Wait(ctx))
	select {
	case err := <-producer:
		require.ErrorIs(t, err, ErrManagerStopped)
	case <-ctx.Done():
		t.Fatal("producer stranded")
	}
	require.Len(t, completed, 2)
	for range 2 {
		require.Error(t, (<-completed).Err)
	}
	require.Empty(t, m.store.items)
	require.Empty(t, m.slots)
	_, err = m.Enqueue(t.Context(), testNotification{id: "notification"})
	require.ErrorIs(t, err, ErrManagerStopped)
}

func TestShutdownDeadlineAbortsDelivery(t *testing.T) {
	target := &blockingTarget{entered: make(chan uuid.UUID, 1), release: make(chan struct{})}
	m, err := NewManager(NewReceivers(NewReceiver("ops", target)), nil)
	require.NoError(t, err)
	require.NoError(t, m.Start(t.Context()))
	_, err = m.Enqueue(t.Context(), testNotification{id: "active"})
	require.NoError(t, err)
	receiveWorkerEntry(t, target.entered)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.Shutdown(ctx), context.DeadlineExceeded)
	wait, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	require.NoError(t, m.Wait(wait))
}

func TestShutdownBeforeStart(t *testing.T) {
	completed := make(chan Completion, 1)
	m, err := NewManager(nil, nil, WithOnComplete(func(c Completion) { completed <- c }))
	require.NoError(t, err)
	_, err = m.Enqueue(t.Context(), testNotification{id: "notification"})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, m.Shutdown(ctx))
	require.ErrorIs(t, (<-completed).Err, ErrManagerStopped)
	require.ErrorIs(t, m.Start(ctx), ErrManagerStopped)
}

func TestConcurrentEnqueueAndShutdown(t *testing.T) {
	completed := make(chan Completion, 100)
	m, err := NewManager(nil, nil, WithQueueCapacity(4), WithWorkers(4), WithOnComplete(func(c Completion) { completed <- c }))
	require.NoError(t, err)
	require.NoError(t, m.Start(t.Context()))
	var wg sync.WaitGroup
	admitted := make(chan uuid.UUID, 100)
	failures := make(chan error, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := m.Enqueue(t.Context(), testNotification{id: "notification"})
			if err == nil {
				admitted <- id
			} else if !errors.Is(err, ErrManagerStopped) {
				failures <- err
			}
		}()
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, m.Shutdown(ctx))
	wg.Wait()
	require.Empty(t, failures)
	require.Equal(t, len(admitted), len(completed))
	require.Empty(t, m.store.items)
}

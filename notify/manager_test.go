package notify

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewManager tests expected behavior.
func TestNewManager(t *testing.T) {
	t.Parallel()

	t.Run("constructs with default logger", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, nil)
		require.NoError(t, err)
		assert.NotNil(t, manager)
	})

	t.Run("constructs with empty receivers", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.Empty(t, manager.receivers)
	})

	t.Run("configures workers", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger(), WithWorkers(3))
		require.NoError(t, err)
		assert.Equal(t, 3, manager.workers)
	})

	t.Run("ignores invalid worker count", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger(), WithWorkers(0), nil)
		require.NoError(t, err)
		assert.Equal(t, 1, manager.workers)
	})
}

// TestManagerEnqueue tests expected behavior.
func TestManagerEnqueue(t *testing.T) {
	t.Parallel()

	t.Run("nil manager errors", func(t *testing.T) {
		t.Parallel()

		var manager *Manager
		id, err := manager.Enqueue(context.Background(), testNotification{id: "n1"})
		require.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("nil context errors", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		id, err := manager.Enqueue(nil, testNotification{id: "n1"})
		require.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("nil notification errors", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		id, err := manager.Enqueue(context.Background(), nil)
		require.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("stores and queues notification", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)

		id, err := manager.Enqueue(context.Background(), testNotification{id: "n1"})
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		queued := <-manager.mailbox
		assert.Equal(t, id, queued)
		n, ok := manager.store.Get(id)
		require.True(t, ok)
		assert.Equal(t, "n1", n.ID())
	})

	t.Run("removes stored notification when context cancels before enqueue", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		manager.mailbox = make(chan string)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		id, err := manager.Enqueue(ctx, testNotification{id: "n1"})
		require.Error(t, err)
		assert.Empty(t, id)
	})
}

// TestManagerReceivers tests expected behavior.
func TestManagerReceivers(t *testing.T) {
	t.Parallel()

	t.Run("nil manager returns nil", func(t *testing.T) {
		t.Parallel()

		var manager *Manager
		assert.Nil(t, manager.Receivers())
	})

	t.Run("returns receivers sorted by name", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(Receivers{
			"z": {Name: "z"},
			"a": {Name: "a"},
		}, testLogger())
		require.NoError(t, err)

		out := manager.Receivers()
		require.Len(t, out, 2)
		assert.Equal(t, "a", out[0].Name)
		assert.Equal(t, "z", out[1].Name)
	})

	t.Run("skips nil receivers", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(Receivers{
			"ops": {Name: "ops"},
			"nil": nil,
		}, testLogger())
		require.NoError(t, err)

		out := manager.Receivers()
		require.Len(t, out, 1)
		assert.Equal(t, "ops", out[0].Name)
	})
}

// TestManagerStart tests expected behavior.
func TestManagerStart(t *testing.T) {
	t.Parallel()

	t.Run("nil manager errors", func(t *testing.T) {
		t.Parallel()

		var manager *Manager
		err := manager.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("nil context errors", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		err = manager.Start(nil)
		require.Error(t, err)
	})

	t.Run("starts dispatcher", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)
	})

	t.Run("errors when started twice", func(t *testing.T) {
		t.Parallel()

		manager, err := NewManager(nil, testLogger())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)

		err = manager.Start(ctx)
		require.ErrorIs(t, err, ErrManagerStarted)
	})

	t.Run("processes queued notifications concurrently with multiple workers", func(t *testing.T) {
		t.Parallel()

		target := &blockingTarget{
			entered: make(chan string, 2),
			release: make(chan struct{}),
		}
		manager, err := NewManager(
			Receivers{"ops": NewReceiver("ops", target)},
			testLogger(),
			WithWorkers(2),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		err = manager.Start(ctx)
		require.NoError(t, err)

		_, err = manager.Enqueue(ctx, testNotification{id: "n1"})
		require.NoError(t, err)
		_, err = manager.Enqueue(ctx, testNotification{id: "n2"})
		require.NoError(t, err)

		seen := map[string]bool{
			receiveWorkerEntry(t, target.entered): true,
			receiveWorkerEntry(t, target.entered): true,
		}
		assert.Equal(t, map[string]bool{"n1": true, "n2": true}, seen)

		close(target.release)
	})
}

// TestNextEventID tests expected behavior.
func TestNextEventID(t *testing.T) {
	t.Parallel()

	t.Run("returns unique uuidv7 ids", func(t *testing.T) {
		t.Parallel()

		first, err := nextQueueID()
		require.NoError(t, err)
		second, err := nextQueueID()
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), first)
	})
}

// blockingTarget records when delivery starts and blocks until released.
type blockingTarget struct {
	entered chan string
	release chan struct{}
}

// Send records the notification ID and waits for release or cancellation.
func (t *blockingTarget) Send(ctx context.Context, payload Payload) (DeliveryResult, error) {
	select {
	case t.entered <- payload.ID():
	case <-ctx.Done():
		return DeliveryResult{}, ctx.Err()
	}

	select {
	case <-t.release:
		return DeliveryResult{Status: "sent"}, nil
	case <-ctx.Done():
		return DeliveryResult{}, ctx.Err()
	}
}

// Type returns the target type.
func (t *blockingTarget) Type() string { return "blocking" }

// receiveWorkerEntry waits for a worker to enter target delivery.
func receiveWorkerEntry(t *testing.T, entered <-chan string) string {
	t.Helper()

	select {
	case id := <-entered:
		return id
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker delivery")
		return ""
	}
}

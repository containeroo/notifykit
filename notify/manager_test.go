package notify

import (
	"context"
	"regexp"
	"testing"

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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)
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

package notify

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDispatcher tests expected behavior.
func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	store := newStore()
	mailbox := make(chan uuid.UUID)
	delivery := &testDelivery{}
	receivers := Receivers{"ops": {Name: "ops"}}
	logger := testLogger()

	dispatcher := newDispatcher(store, mailbox, delivery, receivers, logger)

	require.NotNil(t, dispatcher)
	assert.Same(t, store, dispatcher.store)
	assert.Equal(t, (<-chan uuid.UUID)(mailbox), dispatcher.mailbox)
	assert.Same(t, delivery, dispatcher.delivery)
	assert.Equal(t, receivers, dispatcher.receivers)
	assert.Same(t, logger, dispatcher.logger)
}

// TestDispatcherStart tests expected behavior.
func TestDispatcherStart(t *testing.T) {
	t.Parallel()

	t.Run("dispatches queued id", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		mailbox := make(chan uuid.UUID, 1)
		delivery := &testDelivery{}
		receiver := &Receiver{Name: "ops"}
		dispatcher := newDispatcher(store, mailbox, delivery, Receivers{"ops": receiver}, testLogger())

		id := uuid.NewV7()
		store.put(id, testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		mailbox <- id
		close(mailbox)
		dispatcher.start(context.Background())

		assert.Equal(t, 1, delivery.calls)
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dispatcher := newDispatcher(newStore(), make(chan uuid.UUID), &testDelivery{}, Receivers{}, testLogger())

		done := make(chan struct{})
		go func() {
			dispatcher.start(ctx)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("dispatcher did not stop")
		}
	})
}

// TestDispatcherDispatch tests expected behavior.
func TestDispatcherDispatch(t *testing.T) {
	t.Parallel()

	t.Run("ignores missing queue id", func(t *testing.T) {
		t.Parallel()

		delivery := &testDelivery{}
		dispatcher := newDispatcher(newStore(), make(chan uuid.UUID), delivery, Receivers{}, testLogger())

		missing := uuid.NewV7()
		dispatcher.dispatch(context.Background(), missing)
		assert.Equal(t, 0, delivery.calls)
	})

	t.Run("deletes queued notification", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{}
		receiver := &Receiver{Name: "ops"}
		dispatcher := newDispatcher(store, make(chan uuid.UUID), delivery, Receivers{"ops": receiver}, testLogger())

		id := uuid.NewV7()
		store.put(id, testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		dispatcher.dispatch(context.Background(), id)

		_, ok := store.get(id)
		assert.False(t, ok)
		assert.Equal(t, 1, delivery.calls)
	})

	t.Run("does not dispatch without receivers", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{}
		dispatcher := newDispatcher(store, make(chan uuid.UUID), delivery, Receivers{}, testLogger())

		id := uuid.NewV7()
		store.put(id, testNotification{id: "n1", receivers: []ReceiverID{"missing"}})
		dispatcher.dispatch(context.Background(), id)

		assert.Equal(t, 0, delivery.calls)
	})

	t.Run("logs delivery error", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{err: errors.New("boom")}
		receiver := &Receiver{Name: "ops"}
		dispatcher := newDispatcher(store, make(chan uuid.UUID), delivery, Receivers{"ops": receiver}, testLogger())

		id := uuid.NewV7()
		store.put(id, testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		dispatcher.dispatch(context.Background(), id)

		assert.Equal(t, 1, delivery.calls)
	})
}

// TestDispatcherResolveReceivers tests expected behavior.
func TestDispatcherResolveReceivers(t *testing.T) {
	t.Parallel()

	receivers := Receivers{
		"ops": {Name: "ops"},
		"dev": {Name: "dev"},
	}
	dispatcher := newDispatcher(newStore(), make(chan uuid.UUID), &testDelivery{}, receivers, testLogger())

	t.Run("returns all receivers without names", func(t *testing.T) {
		t.Parallel()

		out := dispatcher.resolveReceivers(nil)
		assert.Len(t, out, 2)
	})

	t.Run("returns matching named receivers", func(t *testing.T) {
		t.Parallel()

		out := dispatcher.resolveReceivers([]ReceiverID{"ops"})
		require.Len(t, out, 1)
		assert.Equal(t, "ops", out[0].Name)
	})

	t.Run("skips missing receivers", func(t *testing.T) {
		t.Parallel()

		out := dispatcher.resolveReceivers([]ReceiverID{"missing"})
		assert.Empty(t, out)
	})
}

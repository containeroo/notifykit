package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDispatcher tests expected behavior.
func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("requires store", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(nil, make(chan string), &testDelivery{}, nil, testLogger())
		require.Error(t, err)
		assert.Nil(t, dispatcher)
	})

	t.Run("requires mailbox", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(newStore(), nil, &testDelivery{}, nil, testLogger())
		require.Error(t, err)
		assert.Nil(t, dispatcher)
	})

	t.Run("requires delivery", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(newStore(), make(chan string), nil, nil, testLogger())
		require.Error(t, err)
		assert.Nil(t, dispatcher)
	})

	t.Run("constructs dispatcher with default logger", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(newStore(), make(chan string), &testDelivery{}, nil, nil)
		require.NoError(t, err)
		assert.NotNil(t, dispatcher)
	})

	t.Run("constructs dispatcher with empty receivers", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(newStore(), make(chan string), &testDelivery{}, nil, testLogger())
		require.NoError(t, err)
		assert.NotNil(t, dispatcher)
		assert.Empty(t, dispatcher.receivers)
	})
}

// TestDispatcherStart tests expected behavior.
func TestDispatcherStart(t *testing.T) {
	t.Parallel()

	t.Run("nil dispatcher returns", func(t *testing.T) {
		t.Parallel()

		var dispatcher *dispatcher
		dispatcher.start(context.Background())
	})

	t.Run("dispatches queued id", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		mailbox := make(chan string, 1)
		delivery := &testDelivery{}
		receiver := &Receiver{Name: "ops"}
		dispatcher, err := newDispatcher(store, mailbox, delivery, Receivers{"ops": receiver}, testLogger())
		require.NoError(t, err)

		store.put("q1", testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		mailbox <- "q1"
		close(mailbox)
		dispatcher.start(context.Background())

		assert.Equal(t, 1, delivery.calls)
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dispatcher, err := newDispatcher(newStore(), make(chan string), &testDelivery{}, nil, testLogger())
		require.NoError(t, err)

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
		dispatcher, err := newDispatcher(newStore(), make(chan string), delivery, nil, testLogger())
		require.NoError(t, err)

		dispatcher.dispatch(context.Background(), "missing")
		assert.Equal(t, 0, delivery.calls)
	})

	t.Run("deletes queued notification", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{}
		receiver := &Receiver{Name: "ops"}
		dispatcher, err := newDispatcher(store, make(chan string), delivery, Receivers{"ops": receiver}, testLogger())
		require.NoError(t, err)

		store.put("q1", testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		dispatcher.dispatch(context.Background(), "q1")

		_, ok := store.get("q1")
		assert.False(t, ok)
		assert.Equal(t, 1, delivery.calls)
	})

	t.Run("does not dispatch without receivers", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{}
		dispatcher, err := newDispatcher(store, make(chan string), delivery, Receivers{}, testLogger())
		require.NoError(t, err)

		store.put("q1", testNotification{id: "n1", receivers: []ReceiverID{"missing"}})
		dispatcher.dispatch(context.Background(), "q1")

		assert.Equal(t, 0, delivery.calls)
	})

	t.Run("logs delivery error", func(t *testing.T) {
		t.Parallel()

		store := newStore()
		delivery := &testDelivery{err: errors.New("boom")}
		receiver := &Receiver{Name: "ops"}
		dispatcher, err := newDispatcher(store, make(chan string), delivery, Receivers{"ops": receiver}, testLogger())
		require.NoError(t, err)

		store.put("q1", testNotification{id: "n1", receivers: []ReceiverID{"ops"}})
		dispatcher.dispatch(context.Background(), "q1")

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
	dispatcher, err := newDispatcher(newStore(), make(chan string), &testDelivery{}, receivers, testLogger())
	require.NoError(t, err)

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

	t.Run("skips nil receivers", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcher(newStore(), make(chan string), &testDelivery{}, Receivers{"nil": nil}, testLogger())
		require.NoError(t, err)

		out := dispatcher.resolveReceivers(nil)
		assert.Empty(t, out)
	})
}

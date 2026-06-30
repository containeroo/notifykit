package notify

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewReceiver tests expected behavior.
func TestNewReceiver(t *testing.T) {
	t.Parallel()

	t.Run("constructs receiver with id name and targets", func(t *testing.T) {
		t.Parallel()

		target := &testTarget{}

		receiver := NewReceiver("ops", target)

		require.NotNil(t, receiver)
		assert.Equal(t, ReceiverID("ops"), receiver.ID)
		assert.Equal(t, "ops", receiver.Name)
		assert.Equal(t, []Target{target}, receiver.Targets)
	})
}

// TestNewReceivers tests expected behavior.
func TestNewReceivers(t *testing.T) {
	t.Parallel()

	t.Run("builds receiver map", func(t *testing.T) {
		t.Parallel()

		receivers := NewReceivers(NewReceiver("ops"), nil, NewReceiver("dev").WithName("Developers"))

		require.Len(t, receivers, 2)
		assert.Equal(t, ReceiverID("ops"), receivers["ops"].ID)
		assert.Equal(t, "ops", receivers["ops"].Name)
		assert.Equal(t, ReceiverID("dev"), receivers["dev"].ID)
		assert.Equal(t, "Developers", receivers["dev"].Name)
	})

	t.Run("last duplicate wins", func(t *testing.T) {
		t.Parallel()

		receivers := NewReceivers(NewReceiver("ops").WithName("first"), NewReceiver("ops").WithName("second"))

		require.Len(t, receivers, 1)
		assert.Equal(t, "second", receivers["ops"].Name)
	})
}

// TestSendTo tests expected behavior.
func TestSendTo(t *testing.T) {
	t.Parallel()

	t.Run("sends to receiver", func(t *testing.T) {
		t.Parallel()

		target := &testTarget{}

		err := SendTo(context.Background(), testNotification{id: "n1"}, NewReceiver("ops", target))

		require.NoError(t, err)
		assert.Equal(t, 1, target.calls)
		assert.Equal(t, "ops", target.payload.Receiver)
	})

	t.Run("honors receiver routing", func(t *testing.T) {
		t.Parallel()

		opsTarget := &testTarget{}
		devTarget := &testTarget{}

		err := SendTo(
			context.Background(),
			testNotification{id: "n1", receivers: []ReceiverID{"dev"}},
			NewReceiver("ops", opsTarget),
			NewReceiver("dev", devTarget),
		)

		require.NoError(t, err)
		assert.Equal(t, 0, opsTarget.calls)
		assert.Equal(t, 1, devTarget.calls)
	})

	t.Run("requires resolved receivers", func(t *testing.T) {
		t.Parallel()

		err := SendTo(context.Background(), testNotification{id: "n1"})

		require.Error(t, err)
	})
}

// TestReceiverBuilderMethods tests expected behavior.
func TestReceiverBuilderMethods(t *testing.T) {
	t.Parallel()

	t.Run("sets fields fluently", func(t *testing.T) {
		t.Parallel()

		first := &testTarget{}
		second := &testTarget{}
		customData := map[string]any{"team": "platform"}
		retry := RetryConfig{Count: 2, Backoff: time.Second}

		receiver := NewReceiver("ops", first).
			WithName("Operations").
			WithCustomData(customData).
			WithRetry(retry).
			WithTargets(second)

		assert.Equal(t, "Operations", receiver.Name)
		assert.Equal(t, customData, receiver.CustomData)
		assert.Equal(t, retry, receiver.Retry)
		assert.Equal(t, []Target{first, second}, receiver.Targets)
	})

	t.Run("nil receiver methods are safe", func(t *testing.T) {
		t.Parallel()

		var receiver *Receiver

		assert.Nil(t, receiver.WithName("ops"))
		assert.Nil(t, receiver.WithCustomData(map[string]any{}))
		assert.Nil(t, receiver.WithRetry(RetryConfig{}))
		assert.Nil(t, receiver.WithTargets(&testTarget{}))
	})
}

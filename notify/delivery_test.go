package notify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDelivery tests expected behavior.
func TestNewDelivery(t *testing.T) {
	t.Parallel()

	t.Run("constructs delivery with default logger", func(t *testing.T) {
		t.Parallel()

		delivery := NewDelivery(nil)
		assert.NotNil(t, delivery)
	})

	t.Run("constructs delivery with provided logger", func(t *testing.T) {
		t.Parallel()

		delivery := NewDelivery(testLogger())
		assert.NotNil(t, delivery)
	})
}

// TestDeliveryEngineDispatch tests expected behavior.
func TestDeliveryEngineDispatch(t *testing.T) {
	t.Parallel()

	t.Run("nil delivery errors", func(t *testing.T) {
		t.Parallel()

		var delivery *deliveryEngine
		err := delivery.Dispatch(context.Background(), Payload{}, nil)
		require.Error(t, err)
	})

	t.Run("sends to receiver target", func(t *testing.T) {
		t.Parallel()

		delivery := NewDelivery(testLogger())

		target := &testTarget{}
		n := testNotification{id: "n1"}
		receiver := &Receiver{Name: "ops", Vars: map[string]any{"team": "platform"}, Targets: []Target{target}}

		err := delivery.Dispatch(context.Background(), Payload{Notification: n}, []*Receiver{receiver})
		require.NoError(t, err)
		assert.Equal(t, 1, target.calls)
		assert.Equal(t, "ops", target.payload.Receiver)
		assert.Equal(t, map[string]any{"team": "platform"}, target.payload.Vars)
	})

	t.Run("joins receiver errors", func(t *testing.T) {
		t.Parallel()

		delivery := NewDelivery(testLogger())

		boom := errors.New("boom")
		target := &testTarget{err: boom}
		receiver := &Receiver{Name: "ops", Targets: []Target{target}}

		err := delivery.Dispatch(context.Background(), Payload{Notification: testNotification{id: "n1"}}, []*Receiver{receiver, nil})
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "receiver is nil")
	})
}

// TestDeliveryEngineDispatchReceiver tests expected behavior.
func TestDeliveryEngineDispatchReceiver(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver errors", func(t *testing.T) {
		t.Parallel()

		delivery := &deliveryEngine{logger: testLogger()}
		err := delivery.dispatchReceiver(context.Background(), nil, Payload{})
		require.Error(t, err)
	})

	t.Run("nil target errors", func(t *testing.T) {
		t.Parallel()

		delivery := &deliveryEngine{logger: testLogger()}
		err := delivery.dispatchReceiver(context.Background(), &Receiver{Name: "ops", Targets: []Target{nil}}, Payload{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target is nil")
	})

	t.Run("uses result target when available", func(t *testing.T) {
		t.Parallel()

		delivery := &deliveryEngine{logger: testLogger()}
		target := &testResultTarget{result: DeliveryResult{Status: "ok"}}
		receiver := &Receiver{Name: "ops", Targets: []Target{target}}

		err := delivery.dispatchReceiver(context.Background(), receiver, Payload{Notification: testNotification{id: "n1"}})
		require.NoError(t, err)
		assert.Equal(t, 1, target.calls)
		assert.Equal(t, "ops", target.payload.Receiver)
	})

	t.Run("does not log delivery response on success", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		delivery := &deliveryEngine{logger: slog.New(slog.NewTextHandler(&logs, nil))}
		target := &testResultTarget{result: DeliveryResult{Status: "sent", StatusCode: 200, Response: "secret-response-token"}}
		receiver := &Receiver{Name: "ops", Targets: []Target{target}}

		err := delivery.dispatchReceiver(context.Background(), receiver, Payload{Notification: testNotification{id: "n1"}})
		require.NoError(t, err)
		assert.Contains(t, logs.String(), "notification target delivered")
		assert.NotContains(t, logs.String(), "secret-response-token")
	})

	t.Run("does not log delivery response on failure", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		delivery := &deliveryEngine{logger: slog.New(slog.NewTextHandler(&logs, nil))}
		target := &testResultTarget{
			testTarget: testTarget{err: errors.New("boom")},
			result:     DeliveryResult{Status: "failed", StatusCode: 500, Response: "secret-response-token"},
		}
		receiver := &Receiver{Name: "ops", Targets: []Target{target}}

		err := delivery.dispatchReceiver(context.Background(), receiver, Payload{Notification: testNotification{id: "n1"}})
		require.Error(t, err)
		assert.Contains(t, logs.String(), "notification target failed")
		assert.NotContains(t, logs.String(), "secret-response-token")
	})
}

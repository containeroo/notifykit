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

	logger := testLogger()
	delivery := newDelivery(logger)

	require.NotNil(t, delivery)
	assert.Same(t, logger, delivery.logger)
}

// TestDeliveryEngineDispatch tests expected behavior.
func TestDeliveryEngineDispatch(t *testing.T) {
	t.Parallel()

	t.Run("sends to receiver target", func(t *testing.T) {
		t.Parallel()

		delivery := newDelivery(testLogger())
		target := &testTarget{}
		n := testNotification{id: "n1"}
		receiver := &Receiver{Name: "ops", CustomData: map[string]any{"team": "platform"}, Targets: []Target{target}}

		err := delivery.dispatch(context.Background(), Payload{Notification: n}, []*Receiver{receiver})
		require.NoError(t, err)
		assert.Equal(t, 1, target.calls)
		assert.Equal(t, "ops", target.payload.Receiver)
		assert.Equal(t, map[string]any{"team": "platform"}, target.payload.CustomData)
	})

	t.Run("joins receiver errors", func(t *testing.T) {
		t.Parallel()

		delivery := newDelivery(testLogger())
		firstErr := errors.New("first")
		secondErr := errors.New("second")
		first := &Receiver{Name: "first", Targets: []Target{&testTarget{err: firstErr}}}
		second := &Receiver{Name: "second", Targets: []Target{&testTarget{err: secondErr}}}

		err := delivery.dispatch(context.Background(), Payload{Notification: testNotification{id: "n1"}}, []*Receiver{first, second})

		require.Error(t, err)
		assert.ErrorIs(t, err, firstErr)
		assert.ErrorIs(t, err, secondErr)
	})
}

// TestDeliveryEngineDispatchReceiver tests expected behavior.
func TestDeliveryEngineDispatchReceiver(t *testing.T) {
	t.Parallel()

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
	t.Run("adds target context to attempt logs", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		delivery := &deliveryEngine{logger: logger}
		target := &testTarget{targetType: "email"}
		receiver := &Receiver{ID: "smtp", Name: "SMTP", Targets: []Target{target}}

		err := delivery.dispatchReceiver(context.Background(), receiver, Payload{Notification: testNotification{id: "n1"}})
		require.NoError(t, err)

		output := logs.String()
		assert.Contains(t, output, `msg="notification target attempt"`)
		assert.Contains(t, output, `receiver=SMTP`)
		assert.Contains(t, output, `targetType=email`)
		assert.Contains(t, output, "notificationID="+testNotification{id: "n1"}.ID().String())
		assert.Contains(t, output, `targetIndex=0`)
		assert.Contains(t, output, `attempt=1`)
	})

}

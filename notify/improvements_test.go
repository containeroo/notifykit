package notify

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRoutingAndStructuredErrors(t *testing.T) {
	failure := errors.New("unavailable")
	target := &testTarget{err: failure, result: DeliveryResult{StatusCode: 503}}
	err := SendTo(t.Context(), testNotification{receivers: []ReceiverID{"ops", "ops"}}, NewReceiver("ops", target))
	require.Equal(t, 1, target.calls)
	var detail *DeliveryError
	require.ErrorAs(t, err, &detail)
	require.ErrorIs(t, err, failure)
	require.Equal(t, ReceiverID("ops"), detail.ReceiverID)
	require.Equal(t, "test", detail.TargetType)
	require.Equal(t, 0, detail.TargetIndex)
	require.Equal(t, 1, detail.Attempts)
	require.Equal(t, 503, detail.Result.StatusCode)
	require.Error(t, SendTo(t.Context(), testNotification{}, NewReceiver("empty")))
}

func TestReceiverSnapshots(t *testing.T) {
	target := &testTarget{}
	source := NewReceiver("ops", target).WithCustomData(map[string]any{"team": "original"})
	m, err := NewManager(NewReceivers(source), nil)
	require.NoError(t, err)
	source.CustomData["team"] = "changed"
	snapshot := m.Receivers()[0]
	snapshot.Name = "changed"
	snapshot.CustomData["team"] = "changed"
	snapshot.Targets[0] = nil
	fresh := m.Receivers()[0]
	require.Equal(t, "ops", fresh.Name)
	require.Equal(t, "original", fresh.CustomData["team"])
	require.Same(t, target, fresh.Targets[0])
}

func TestQueueCapacityAndCompletion(t *testing.T) {
	completed := make(chan Completion, 2)
	failure := errors.New("delivery failed")
	m, err := NewManager(NewReceivers(NewReceiver("ops", &testTarget{err: failure})), nil, WithQueueCapacity(1), WithOnComplete(func(c Completion) { completed <- c }))
	require.NoError(t, err)
	id, err := m.Enqueue(t.Context(), testNotification{id: "n1"})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err = m.Enqueue(ctx, testNotification{id: "n2"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	m.store.mu.RLock()
	require.Len(t, m.store.items, 1)
	m.store.mu.RUnlock()
	require.NoError(t, m.Start(t.Context()))
	select {
	case result := <-completed:
		require.Equal(t, id, result.QueueID)
		require.Equal(t, "n1", result.NotificationID)
		require.ErrorIs(t, result.Err, failure)
	case <-time.After(time.Second):
		t.Fatal("missing completion")
	}
	_, err = m.Enqueue(t.Context(), testNotification{id: "n3"})
	require.NoError(t, err)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("slot was not released")
	}
	_, err = NewManager(nil, nil, WithQueueCapacity(0))
	require.Error(t, err)
}

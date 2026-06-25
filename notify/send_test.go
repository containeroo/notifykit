package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSend tests expected behavior.
func TestSend(t *testing.T) {
	t.Parallel()

	t.Run("requires context", func(t *testing.T) {
		t.Parallel()

		err := Send(nil, testNotification{id: "n1"}, nil, testLogger())
		require.Error(t, err)
	})

	t.Run("requires notification", func(t *testing.T) {
		t.Parallel()

		err := Send(context.Background(), nil, nil, testLogger())
		require.Error(t, err)
	})

	t.Run("requires resolved receivers", func(t *testing.T) {
		t.Parallel()

		err := Send(context.Background(), testNotification{id: "n1", receivers: []ReceiverID{"missing"}}, nil, testLogger())
		require.Error(t, err)
	})

	t.Run("sends to named receiver", func(t *testing.T) {
		t.Parallel()

		target := &testTarget{}
		receivers := Receivers{
			"ops": {Name: "ops", Vars: map[string]any{"team": "platform"}, Targets: []Target{target}},
			"dev": {Name: "dev", Targets: []Target{&testTarget{}}},
		}

		err := Send(context.Background(), testNotification{id: "n1", receivers: []ReceiverID{"ops"}}, receivers, nil)

		require.NoError(t, err)
		assert.Equal(t, 1, target.calls)
		assert.Equal(t, "ops", target.payload.Receiver)
		assert.Equal(t, map[string]any{"team": "platform"}, target.payload.Vars)
	})

	t.Run("sends to all receivers without names", func(t *testing.T) {
		t.Parallel()

		first := &testTarget{}
		second := &testTarget{}
		receivers := Receivers{
			"ops": {Name: "ops", Targets: []Target{first}},
			"dev": {Name: "dev", Targets: []Target{second}},
		}

		err := Send(context.Background(), testNotification{id: "n1"}, receivers, testLogger())

		require.NoError(t, err)
		assert.Equal(t, 1, first.calls)
		assert.Equal(t, 1, second.calls)
	})

	t.Run("returns delivery error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		receivers := Receivers{
			"ops": {Name: "ops", Targets: []Target{&testTarget{err: boom}}},
		}

		err := Send(context.Background(), testNotification{id: "n1", receivers: []ReceiverID{"ops"}}, receivers, testLogger())

		require.ErrorIs(t, err, boom)
	})
}

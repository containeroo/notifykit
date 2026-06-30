package notify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPayloadData tests expected behavior.
func TestPayloadData(t *testing.T) {
	t.Parallel()

	t.Run("nil notification returns nil", func(t *testing.T) {
		t.Parallel()

		payload := Payload{}
		assert.Nil(t, payload.Data("subject"))
	})

	t.Run("delegates to notification", func(t *testing.T) {
		t.Parallel()

		payload := Payload{
			Notification: testNotification{id: "n1"},
			Receiver:     "ops",
			CustomData:   map[string]any{"team": "platform"},
		}
		data := payload.Data("hello")
		assert.Equal(t, map[string]any{"receiver": "ops", "CustomData": map[string]any{"team": "platform"}, "subject": "hello"}, data)
	})
}

// TestPayloadID tests expected behavior.
func TestPayloadID(t *testing.T) {
	t.Parallel()

	t.Run("nil notification returns empty", func(t *testing.T) {
		t.Parallel()

		payload := Payload{}
		assert.Empty(t, payload.ID())
	})

	t.Run("returns notification id", func(t *testing.T) {
		t.Parallel()

		payload := Payload{Notification: testNotification{id: "n1"}}
		assert.Equal(t, "n1", payload.ID())
	})
}

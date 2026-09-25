package notify

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStore tests expected behavior.
func TestNewStore(t *testing.T) {
	t.Parallel()

	s := newStore()
	require.NotNil(t, s)
	assert.NotNil(t, s.items)
}

// TestStorePut tests expected behavior.
func TestStorePut(t *testing.T) {
	t.Parallel()

	s := newStore()
	id := uuid.NewV7()
	s.put(id, testNotification{id: "n1"})
	n, ok := s.get(id)
	require.True(t, ok)
	assert.NotEqual(t, uuid.Nil(), n.ID())
}

// TestStoreGet tests expected behavior.
func TestStoreGet(t *testing.T) {
	t.Parallel()

	s := newStore()
	n, ok := s.get(uuid.NewV7())
	assert.Nil(t, n)
	assert.False(t, ok)
}

// TestStoreDelete tests expected behavior.
func TestStoreDelete(t *testing.T) {
	t.Parallel()

	s := newStore()
	id := uuid.NewV7()
	s.put(id, testNotification{id: "n1"})
	s.delete(id)
	_, ok := s.get(id)
	assert.False(t, ok)
}

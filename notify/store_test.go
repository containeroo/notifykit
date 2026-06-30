package notify

import (
	"testing"

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

	t.Run("ignores invalid input", func(t *testing.T) {
		t.Parallel()

		s := newStore()
		s.put("", testNotification{id: "n1"})
		s.put("q1", nil)
		assert.Empty(t, s.items)

		var nilStore *store
		nilStore.put("q1", testNotification{id: "n1"})
	})

	t.Run("stores notification", func(t *testing.T) {
		t.Parallel()

		s := newStore()
		s.put("q1", testNotification{id: "n1"})
		n, ok := s.get("q1")
		require.True(t, ok)
		assert.Equal(t, "n1", n.ID())
	})
}

// TestStoreGet tests expected behavior.
func TestStoreGet(t *testing.T) {
	t.Parallel()

	t.Run("nil store returns false", func(t *testing.T) {
		t.Parallel()

		var s *store
		n, ok := s.get("q1")
		assert.Nil(t, n)
		assert.False(t, ok)
	})

	t.Run("missing id returns false", func(t *testing.T) {
		t.Parallel()

		s := newStore()
		n, ok := s.get("missing")
		assert.Nil(t, n)
		assert.False(t, ok)
	})
}

// TestStoreDelete tests expected behavior.
func TestStoreDelete(t *testing.T) {
	t.Parallel()

	t.Run("nil store returns", func(t *testing.T) {
		t.Parallel()

		var s *store
		s.delete("q1")
	})

	t.Run("removes notification", func(t *testing.T) {
		t.Parallel()

		s := newStore()
		s.put("q1", testNotification{id: "n1"})
		s.delete("q1")
		_, ok := s.get("q1")
		assert.False(t, ok)
	})
}

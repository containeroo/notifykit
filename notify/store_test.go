package notify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStore tests expected behavior.
func TestNewStore(t *testing.T) {
	t.Parallel()

	store := NewStore()
	require.NotNil(t, store)
	assert.NotNil(t, store.items)
}

// TestStorePut tests expected behavior.
func TestStorePut(t *testing.T) {
	t.Parallel()

	t.Run("ignores invalid input", func(t *testing.T) {
		t.Parallel()

		store := NewStore()
		store.Put("", testNotification{id: "n1"})
		store.Put("q1", nil)
		assert.Empty(t, store.items)

		var nilStore *Store
		nilStore.Put("q1", testNotification{id: "n1"})
	})

	t.Run("stores notification", func(t *testing.T) {
		t.Parallel()

		store := NewStore()
		store.Put("q1", testNotification{id: "n1"})
		n, ok := store.Get("q1")
		require.True(t, ok)
		assert.Equal(t, "n1", n.ID())
	})
}

// TestStoreGet tests expected behavior.
func TestStoreGet(t *testing.T) {
	t.Parallel()

	t.Run("nil store returns false", func(t *testing.T) {
		t.Parallel()

		var store *Store
		n, ok := store.Get("q1")
		assert.Nil(t, n)
		assert.False(t, ok)
	})

	t.Run("missing id returns false", func(t *testing.T) {
		t.Parallel()

		store := NewStore()
		n, ok := store.Get("missing")
		assert.Nil(t, n)
		assert.False(t, ok)
	})
}

// TestStoreDelete tests expected behavior.
func TestStoreDelete(t *testing.T) {
	t.Parallel()

	t.Run("nil store returns", func(t *testing.T) {
		t.Parallel()

		var store *Store
		store.Delete("q1")
	})

	t.Run("removes notification", func(t *testing.T) {
		t.Parallel()

		store := NewStore()
		store.Put("q1", testNotification{id: "n1"})
		store.Delete("q1")
		_, ok := store.Get("q1")
		assert.False(t, ok)
	})
}

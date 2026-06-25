package ids

import (
	"regexp"
	"testing"
)

func TestNewUUIDV7(t *testing.T) {
	t.Parallel()
	t.Run("returns uuidv7 string", func(t *testing.T) {
		t.Parallel()

		id, err := NewUUIDV7()

		require.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), id)
	})

	t.Run("returns different ids", func(t *testing.T) {
		t.Parallel()

		first, err := NewUUIDV7()
		require.NoError(t, err)
		second, err := NewUUIDV7()
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
	})
}

func TestMustNewUUIDV7(t *testing.T) {
	t.Parallel()
	t.Run("returns uuidv7 string", func(t *testing.T) {
		t.Parallel()

		id := MustNewUUIDV7()

		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), id)
	})
}

func TestFormatUUID(t *testing.T) {
	t.Parallel()
	t.Run("formats canonical uuid", func(t *testing.T) {
		t.Parallel()

		raw := [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x80, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd}

		id := formatUUID(raw)

		assert.Equal(t, "01234567-89ab-cdef-8001-23456789abcd", id)
	})
}

package notify

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermanent(t *testing.T) {
	t.Parallel()

	t.Run("preserves nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, Permanent(nil))
	})

	t.Run("marks wrapped error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		err := Permanent(boom)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.True(t, IsPermanent(err))
		assert.False(t, IsTransport(err))
	})

	t.Run("preserves existing permanent error", func(t *testing.T) {
		t.Parallel()

		err := Permanent(errors.New("boom"))
		assert.Equal(t, err, Permanent(err))
	})

	t.Run("overrides transport classification", func(t *testing.T) {
		t.Parallel()

		err := Permanent(Transport(errors.New("boom")))

		assert.True(t, IsPermanent(err))
		assert.False(t, IsTransport(err))
	})
}

func TestTransport(t *testing.T) {
	t.Parallel()

	t.Run("preserves nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, Transport(nil))
	})

	t.Run("marks wrapped error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		err := Transport(boom)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.True(t, IsTransport(err))
		assert.False(t, IsPermanent(err))
	})

	t.Run("preserves existing transport error", func(t *testing.T) {
		t.Parallel()

		err := Transport(errors.New("boom"))
		assert.Equal(t, err, Transport(err))
	})

	t.Run("does not override permanent classification", func(t *testing.T) {
		t.Parallel()

		permanent := Permanent(errors.New("boom"))
		err := Transport(permanent)

		assert.Equal(t, permanent, err)
		assert.True(t, IsPermanent(err))
		assert.False(t, IsTransport(err))
	})
}

package header

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidFieldName(t *testing.T) {
	t.Parallel()

	t.Run("accepts lowercase letters", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldName("x-test"))
	})

	t.Run("accepts uppercase letters", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldName("X-Test"))
	})

	t.Run("accepts digits", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldName("X-Test-123"))
	})

	t.Run("accepts allowed symbols", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldName("!#$%&'*+-.^_`|~"))
	})

	t.Run("rejects empty name", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName(""))
	})

	t.Run("rejects leading whitespace", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName(" X-Test"))
	})

	t.Run("rejects trailing whitespace", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X-Test "))
	})

	t.Run("rejects colon", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X-Test:Bad"))
	})

	t.Run("rejects slash", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X/Test"))
	})

	t.Run("rejects space inside name", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X Test"))
	})

	t.Run("rejects tab inside name", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X\tTest"))
	})

	t.Run("rejects non-ascii character", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldName("X-Ä"))
	})
}

func TestValidFieldNameChar(t *testing.T) {
	t.Parallel()

	t.Run("accepts lowercase letter", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('a'))
	})

	t.Run("accepts uppercase letter", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('Z'))
	})

	t.Run("accepts digit", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('7'))
	})

	t.Run("accepts exclamation mark", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('!'))
	})

	t.Run("accepts hash", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('#'))
	})

	t.Run("accepts dollar", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('$'))
	})

	t.Run("accepts percent", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('%'))
	})

	t.Run("accepts ampersand", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('&'))
	})

	t.Run("accepts apostrophe", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('\''))
	})

	t.Run("accepts asterisk", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('*'))
	})

	t.Run("accepts plus", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('+'))
	})

	t.Run("accepts hyphen", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('-'))
	})

	t.Run("accepts dot", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('.'))
	})

	t.Run("accepts caret", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('^'))
	})

	t.Run("accepts underscore", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('_'))
	})

	t.Run("accepts backtick", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('`'))
	})

	t.Run("accepts pipe", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('|'))
	})

	t.Run("accepts tilde", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidFieldNameChar('~'))
	})

	t.Run("rejects colon", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar(':'))
	})

	t.Run("rejects space", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar(' '))
	})

	t.Run("rejects tab", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar('\t'))
	})

	t.Run("rejects carriage return", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar('\r'))
	})

	t.Run("rejects line feed", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar('\n'))
	})

	t.Run("rejects non-ascii character", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidFieldNameChar('Ä'))
	})
}

func TestContainsNewline(t *testing.T) {
	t.Parallel()

	t.Run("returns false without newline", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ContainsNewline("token"))
	})

	t.Run("returns false for empty value", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ContainsNewline(""))
	})

	t.Run("returns true with carriage return", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ContainsNewline("ok\rInjected: yes"))
	})

	t.Run("returns true with line feed", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ContainsNewline("ok\nInjected: yes"))
	})

	t.Run("returns true with crlf", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ContainsNewline("ok\r\nInjected: yes"))
	})
}

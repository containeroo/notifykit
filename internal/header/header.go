package header

import "strings"

// ValidFieldName reports whether name is a valid HTTP-style header field name.
func ValidFieldName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}

	for _, r := range name {
		if !ValidFieldNameChar(r) {
			return false
		}
	}
	return true
}

// ValidFieldNameChar reports whether r is allowed in a header field name.
func ValidFieldNameChar(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}

	switch r {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

// ContainsNewline reports whether value contains newline characters.
func ContainsNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

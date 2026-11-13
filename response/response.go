// Package response evaluates configurable HTTP response acceptance rules.
// It does not load configuration, perform HTTP requests, or log response bodies.
package response

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxErrorBytes = 256

// Rule specifies accepted HTTP statuses and at most one body matcher.
// With no StatusCodes, HTTP 2xx is accepted. The zero value checks status only.
type Rule struct {
	// StatusCodes optionally overrides the accepted HTTP statuses.
	StatusCodes []int
	// Text validates a plain-text response when non-nil.
	Text *Text
	// JSON validates a JSON response when non-nil.
	JSON *JSON
}

// Text describes one plain-text match; exactly one match must be selected.
type Text struct {
	// Equals requires the entire response body to equal the supplied value.
	Equals *string
	// Contains requires the response body to include the supplied value.
	Contains *string
}

// JSON describes a typed JSON value match and optional error extraction.
type JSON struct {
	// Pointer is an RFC 6901 JSON Pointer to the expected value; empty selects the root.
	Pointer string
	// Equals contains the expected JSON value, including its type.
	Equals json.RawMessage
	// ErrorPointer is an optional RFC 6901 pointer to a scalar error message.
	ErrorPointer string
}

// Clone returns an independent copy of the rule and its mutable fields.
func (r Rule) Clone() Rule {
	copy := Rule{StatusCodes: append([]int(nil), r.StatusCodes...)}
	if r.Text != nil {
		copy.Text = &Text{}
		if r.Text.Equals != nil {
			value := *r.Text.Equals
			copy.Text.Equals = &value
		}
		if r.Text.Contains != nil {
			value := *r.Text.Contains
			copy.Text.Contains = &value
		}
	}
	if r.JSON != nil {
		copy.JSON = &JSON{
			Pointer:      r.JSON.Pointer,
			Equals:       append(json.RawMessage(nil), r.JSON.Equals...),
			ErrorPointer: r.JSON.ErrorPointer,
		}
	}
	return copy
}

// Validate checks that a rule is structurally valid without inspecting a response.
func (r Rule) Validate() error {
	for _, code := range r.StatusCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("invalid HTTP status code %d: expected 100–599", code)
		}
	}

	if r.Text != nil && r.JSON != nil {
		return errors.New("text and JSON response matchers are mutually exclusive")
	}
	if r.Text != nil {
		if (r.Text.Equals == nil) == (r.Text.Contains == nil) {
			return errors.New("text response requires exactly one of equals or contains")
		}
		if r.Text.Contains != nil && *r.Text.Contains == "" {
			return errors.New("text contains must not be empty")
		}
	}
	if r.JSON != nil {
		if err := validatePointer(r.JSON.Pointer); err != nil {
			return fmt.Errorf("invalid JSON pointer: %w", err)
		}
		if len(r.JSON.Equals) == 0 || !json.Valid(r.JSON.Equals) {
			return errors.New("JSON equals must contain a valid JSON value")
		}
		if r.JSON.ErrorPointer != "" {
			if err := validatePointer(r.JSON.ErrorPointer); err != nil {
				return fmt.Errorf("invalid JSON error pointer: %w", err)
			}
		}
	}
	return nil
}

// Evaluate checks the HTTP status and optional body matcher. Error details are
// extracted only through explicitly configured text or JSON matchers.
func (r Rule) Evaluate(statusCode int, body []byte) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid webhook response rule: %w", err)
	}
	if statusCode < 100 || statusCode > 599 {
		return fmt.Errorf("invalid webhook HTTP status %d", statusCode)
	}

	accepted := r.acceptsStatus(statusCode)
	matched, detail := r.evaluateBody(body, accepted)
	if accepted && matched {
		return nil
	}

	return evaluationError(statusCode, accepted, detail)
}

// acceptsStatus checks the configured HTTP statuses, defaulting to 2xx.
func (r Rule) acceptsStatus(statusCode int) bool {
	if len(r.StatusCodes) == 0 {
		return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
	}

	for _, code := range r.StatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// evaluateBody evaluates only the configured body matcher.
func (r Rule) evaluateBody(body []byte, accepted bool) (bool, string) {
	switch {
	case r.Text != nil:
		return r.Text.evaluate(string(body), accepted)
	case r.JSON != nil:
		return r.JSON.evaluate(body, accepted)
	default:
		return true, ""
	}
}

// evaluate compares plain text and opts into reporting a bounded body on failure.
func (t Text) evaluate(body string, accepted bool) (bool, string) {
	matched := false
	if t.Equals != nil {
		matched = body == *t.Equals
	} else {
		matched = strings.Contains(body, *t.Contains)
	}

	if accepted && matched {
		return true, ""
	}
	return matched, safeDetail(body)
}

// evaluate compares typed JSON values and optionally extracts an error field.
func (j JSON) evaluate(body []byte, accepted bool) (bool, string) {
	data, ok := decodeJSON(body)
	if !ok {
		return false, ""
	}
	expected, ok := decodeJSON(j.Equals)
	if !ok {
		return false, ""
	}

	actual, found := lookup(data, j.Pointer)
	matched := found && reflect.DeepEqual(actual, expected)
	if accepted && matched {
		return true, ""
	}
	return matched, j.errorDetail(data)
}

// errorDetail extracts only an explicitly selected scalar JSON error value.
func (j JSON) errorDetail(data any) string {
	if j.ErrorPointer == "" {
		return ""
	}
	value, found := lookup(data, j.ErrorPointer)
	if !found {
		return ""
	}
	return scalarDetail(value)
}

// decodeJSON rejects malformed or trailing data and preserves JSON number types.
func decodeJSON(body []byte) (any, bool) {
	if !json.Valid(body) {
		return nil, false
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

// evaluationError formats a rejected response without exposing arbitrary JSON.
func evaluationError(statusCode int, accepted bool, detail string) error {
	reason := "response did not match the acceptance rule"
	if !accepted {
		reason = "unexpected HTTP status"
	}
	if detail != "" {
		return fmt.Errorf("webhook HTTP %d: %s: %s", statusCode, reason, detail)
	}
	return fmt.Errorf("webhook HTTP %d: %s", statusCode, reason)
}

// HasBodyMatcher reports whether evaluation requires a complete response body.
func (r Rule) HasBodyMatcher() bool {
	return r.Text != nil || r.JSON != nil
}

// validatePointer checks RFC 6901 syntax, including escape sequences.
func validatePointer(pointer string) error {
	if pointer == "" {
		return nil
	}
	if pointer[0] != '/' {
		return errors.New("JSON pointer must start with /")
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		for index := 0; index < len(segment); index++ {
			if segment[index] != '~' {
				continue
			}
			if index+1 == len(segment) || (segment[index+1] != '0' && segment[index+1] != '1') {
				return errors.New("JSON pointer contains invalid ~ escape")
			}
			index++
		}
	}
	return nil
}

// lookup selects an RFC 6901 pointer from parsed JSON, including array indices.
func lookup(data any, pointer string) (any, bool) {
	if pointer == "" {
		return data, true
	}
	for _, raw := range strings.Split(pointer[1:], "/") {
		segment := strings.NewReplacer("~1", "/", "~0", "~").Replace(raw)
		switch value := data.(type) {
		case map[string]any:
			var ok bool
			data, ok = value[segment]
			if !ok {
				return nil, false
			}
		case []any:
			if segment == "" || (len(segment) > 1 && segment[0] == '0') {
				return nil, false
			}
			for _, digit := range segment {
				if digit < '0' || digit > '9' {
					return nil, false
				}
			}
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false
			}
			data = value[index]
		default:
			return nil, false
		}
	}
	return data, true
}

// scalarDetail formats JSON error values without dumping arbitrary objects or arrays.
func scalarDetail(value any) string {
	switch value := value.(type) {
	case string:
		return safeDetail(value)
	case json.Number:
		return safeDetail(value.String())
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

// safeDetail removes control characters and bounds the displayed error text.
func safeDetail(value string) string {
	var out strings.Builder
	for len(value) > 0 && out.Len() < maxErrorBytes {
		r, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		if (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) || r == '\u2028' || r == '\u2029' {
			r = ' '
		}
		if out.Len()+utf8.RuneLen(r) > maxErrorBytes {
			break
		}
		out.WriteRune(r)
	}
	return strings.TrimSpace(out.String())
}

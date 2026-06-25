package ids

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var assert = assertHelper{}
var require = requireHelper{}

type assertHelper struct{}
type requireHelper struct{}

func (assertHelper) Equal(t *testing.T, expected, actual any, msgAndArgs ...any) bool {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("not equal:\nexpected: %#v\nactual:   %#v", expected, actual)
		return false
	}
	return true
}

func (assertHelper) NotEqual(t *testing.T, expected, actual any, msgAndArgs ...any) bool {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		t.Errorf("values are equal: %#v", actual)
		return false
	}
	return true
}

func (assertHelper) Nil(t *testing.T, value any, msgAndArgs ...any) bool {
	t.Helper()
	if !isNil(value) {
		t.Errorf("expected nil, got %#v", value)
		return false
	}
	return true
}

func (assertHelper) NotNil(t *testing.T, value any, msgAndArgs ...any) bool {
	t.Helper()
	if isNil(value) {
		t.Errorf("expected non-nil value")
		return false
	}
	return true
}

func (assertHelper) Empty(t *testing.T, value any, msgAndArgs ...any) bool {
	t.Helper()
	if !isEmptyTestValue(value) {
		t.Errorf("expected empty value, got %#v", value)
		return false
	}
	return true
}

func (assertHelper) NotEmpty(t *testing.T, value any, msgAndArgs ...any) bool {
	t.Helper()
	if isEmptyTestValue(value) {
		t.Errorf("expected non-empty value")
		return false
	}
	return true
}

func (assertHelper) True(t *testing.T, value bool, msgAndArgs ...any) bool {
	t.Helper()
	if !value {
		t.Errorf("expected true")
		return false
	}
	return true
}

func (assertHelper) False(t *testing.T, value bool, msgAndArgs ...any) bool {
	t.Helper()
	if value {
		t.Errorf("expected false")
		return false
	}
	return true
}

func (assertHelper) Len(t *testing.T, value any, length int, msgAndArgs ...any) bool {
	t.Helper()
	got, ok := testLen(value)
	if !ok || got != length {
		t.Errorf("expected length %d, got %d for %#v", length, got, value)
		return false
	}
	return true
}

func (assertHelper) Contains(t *testing.T, s, contains any, msgAndArgs ...any) bool {
	t.Helper()
	if !testContains(s, contains) {
		t.Errorf("expected %#v to contain %#v", s, contains)
		return false
	}
	return true
}

func (assertHelper) NotContains(t *testing.T, s, contains any, msgAndArgs ...any) bool {
	t.Helper()
	if testContains(s, contains) {
		t.Errorf("expected %#v not to contain %#v", s, contains)
		return false
	}
	return true
}

func (assertHelper) ErrorIs(t *testing.T, err, target error, msgAndArgs ...any) bool {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("expected error %v to match %v", err, target)
		return false
	}
	return true
}

func (assertHelper) Regexp(t *testing.T, rx any, value any, msgAndArgs ...any) bool {
	t.Helper()
	var re *regexp.Regexp
	switch v := rx.(type) {
	case *regexp.Regexp:
		re = v
	case string:
		re = regexp.MustCompile(v)
	default:
		t.Errorf("invalid regexp %#v", rx)
		return false
	}
	if !re.MatchString(fmt.Sprint(value)) {
		t.Errorf("expected %q to match %q", value, re.String())
		return false
	}
	return true
}

func (assertHelper) EqualError(t *testing.T, err error, expected string, msgAndArgs ...any) bool {
	t.Helper()
	if err == nil || err.Error() != expected {
		t.Errorf("expected error %q, got %v", expected, err)
		return false
	}
	return true
}

func (assertHelper) JSONEq(t *testing.T, expected, actual string, msgAndArgs ...any) bool {
	t.Helper()
	var e, a any
	if err := json.Unmarshal([]byte(expected), &e); err != nil {
		t.Errorf("invalid expected JSON: %v", err)
		return false
	}
	if err := json.Unmarshal([]byte(actual), &a); err != nil {
		t.Errorf("invalid actual JSON: %v", err)
		return false
	}
	if !reflect.DeepEqual(e, a) {
		t.Errorf("JSON not equal:\nexpected: %s\nactual:   %s", expected, actual)
		return false
	}
	return true
}

func (assertHelper) Less(t *testing.T, a, b any, msgAndArgs ...any) bool {
	t.Helper()
	ai, aok := toInt(a)
	bi, bok := toInt(b)
	if !aok || !bok || ai >= bi {
		t.Errorf("expected %#v to be less than %#v", a, b)
		return false
	}
	return true
}

func (requireHelper) NoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func (requireHelper) Error(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func (requireHelper) ErrorIs(t *testing.T, err, target error, msgAndArgs ...any) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v to match %v", err, target)
	}
}

func (requireHelper) NotNil(t *testing.T, value any, msgAndArgs ...any) {
	t.Helper()
	if isNil(value) {
		t.Fatalf("expected non-nil value")
	}
}

func (requireHelper) True(t *testing.T, value bool, msgAndArgs ...any) {
	t.Helper()
	if !value {
		t.Fatalf("expected true")
	}
}

func (requireHelper) Len(t *testing.T, value any, length int, msgAndArgs ...any) {
	t.Helper()
	got, ok := testLen(value)
	if !ok || got != length {
		t.Fatalf("expected length %d, got %d for %#v", length, got, value)
	}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func isEmptyTestValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return rv.IsNil()
	default:
		return reflect.DeepEqual(value, reflect.Zero(rv.Type()).Interface())
	}
}

func testLen(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len(), true
	default:
		return 0, false
	}
}

func testContains(s, contains any) bool {
	switch v := s.(type) {
	case string:
		return strings.Contains(v, fmt.Sprint(contains))
	case []any:
		for _, item := range v {
			if reflect.DeepEqual(item, contains) {
				return true
			}
		}
		return false
	default:
		rv := reflect.ValueOf(s)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				if reflect.DeepEqual(rv.Index(i).Interface(), contains) {
					return true
				}
			}
		case reflect.Map:
			cv := reflect.ValueOf(contains)
			if cv.IsValid() && cv.Type().AssignableTo(rv.Type().Key()) {
				return rv.MapIndex(cv).IsValid()
			}
		}
	}
	return false
}

func toInt(value any) (int64, bool) {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(rv.Uint()), true
	default:
		return 0, false
	}
}

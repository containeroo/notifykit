package response

import (
	"encoding/json"
	"strings"
	"testing"
)

func str(value string) *string { return &value }

func TestRuleValidate(t *testing.T) {
	t.Parallel()

	t.Run("accepts zero rule", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{}).Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("accepts custom HTTP codes", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{StatusCodes: []int{200, 201, 418}}).Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects invalid HTTP codes", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{StatusCodes: []int{0, 200}}).Validate(); err == nil {
			t.Fatal("expected an invalid status error")
		}
	})

	t.Run("rejects conflicting body matchers", func(t *testing.T) {
		t.Parallel()
		rule := Rule{Text: &Text{Equals: str("ok")}, JSON: &JSON{Equals: json.RawMessage(`true`)}}
		if err := rule.Validate(); err == nil {
			t.Fatal("expected a conflicting matcher error")
		}
	})

	t.Run("rejects missing text matcher", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{}}).Validate(); err == nil {
			t.Fatal("expected a text matcher error")
		}
	})

	t.Run("rejects multiple text matchers", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Equals: str("ok"), Contains: str("o")}}).Validate(); err == nil {
			t.Fatal("expected a conflicting text matcher error")
		}
	})

	t.Run("rejects empty contains", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Contains: str("")}}).Validate(); err == nil {
			t.Fatal("expected a contains error")
		}
	})

	t.Run("permits empty equals", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Equals: str("")}}).Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects invalid JSON matcher", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`not-json`)}}).Validate(); err == nil {
			t.Fatal("expected a JSON value error")
		}
	})

	t.Run("rejects invalid JSON pointer", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{JSON: &JSON{Pointer: "ok", Equals: json.RawMessage(`true`)}}).Validate(); err == nil {
			t.Fatal("expected a JSON pointer error")
		}
	})

	t.Run("rejects invalid JSON pointer escape", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{JSON: &JSON{Pointer: "/a~2b", Equals: json.RawMessage(`true`)}}).Validate(); err == nil {
			t.Fatal("expected a JSON pointer escape error")
		}
	})

	t.Run("rejects invalid error pointer", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`), ErrorPointer: "bad"}}).Validate(); err == nil {
			t.Fatal("expected an error pointer validation error")
		}
	})
}

func TestRuleEvaluate(t *testing.T) {
	t.Parallel()

	t.Run("defaults to accepting all HTTP 2xx", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{}).Evaluate(204, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("defaults to rejecting non 2xx", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{}).Evaluate(403, []byte("secret")); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("accepts explicit non 2xx HTTP code", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{StatusCodes: []int{202, 418}}).Evaluate(418, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects other 2xx when overridden", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{StatusCodes: []int{201}}).Evaluate(200, nil); err == nil {
			t.Fatal("expected a status mismatch")
		}
	})

	t.Run("matches exact text", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Equals: str("ok")}}).Evaluate(200, []byte("ok")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("does not trim exact text", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Equals: str("ok")}}).Evaluate(200, []byte("ok\n")); err == nil {
			t.Fatal("expected exact text mismatch")
		}
	})

	t.Run("matches substring", func(t *testing.T) {
		t.Parallel()
		if err := (Rule{Text: &Text{Contains: str("success")}}).Evaluate(200, []byte("request success")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reports opted in plain text errors", func(t *testing.T) {
		t.Parallel()
		err := (Rule{Text: &Text{Equals: str("ok")}}).Evaluate(500, []byte("failure: bad key"))
		if err == nil || !strings.Contains(err.Error(), "failure: bad key") {
			t.Fatalf("expected error detail, got: %v", err)
		}
	})

	t.Run("matches Slack typed JSON boolean", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`), ErrorPointer: "/error"}}
		if err := rule.Evaluate(200, []byte(`{"ok":true}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects JSON boolean as string", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`)}}
		if err := rule.Evaluate(200, []byte(`{"ok":"true"}`)); err == nil {
			t.Fatal("expected JSON type mismatch")
		}
	})

	t.Run("extracts Slack error on HTTP 200", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`), ErrorPointer: "/error"}}
		err := rule.Evaluate(200, []byte(`{"ok":false,"error":"channel_not_found","secret":"private"}`))
		if err == nil || !strings.Contains(err.Error(), "channel_not_found") || strings.Contains(err.Error(), "private") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("extracts nested error on HTTP 403", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/success", Equals: json.RawMessage(`true`), ErrorPointer: "/error/details/message"}}
		err := rule.Evaluate(403, []byte(`{"success":false,"error":{"details":{"message":"invalid_auth"}}}`))
		if err == nil || !strings.Contains(err.Error(), "invalid_auth") || !strings.Contains(err.Error(), "403") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("selects nested array index", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/results/0/status", Equals: json.RawMessage(`"sent"`)}}
		if err := rule.Evaluate(200, []byte(`{"results":[{"status":"sent"}]}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("selects escaped JSON keys", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/a~1b/~0ok", Equals: json.RawMessage(`true`)}}
		if err := rule.Evaluate(200, []byte(`{"a/b":{"~ok":true}}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects absent field", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`)}}
		if err := rule.Evaluate(200, []byte(`{"something":true}`)); err == nil {
			t.Fatal("expected absent field error")
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`)}}
		if err := rule.Evaluate(200, []byte(`{"ok":`)); err == nil {
			t.Fatal("expected malformed JSON error")
		}
	})

	t.Run("supports JSON null root", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Equals: json.RawMessage(`null`)}}
		if err := rule.Evaluate(200, []byte(`null`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("does not report JSON object as an error message", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`), ErrorPointer: "/error"}}
		err := rule.Evaluate(200, []byte(`{"ok":false,"error":{"secret":"private"}}`))
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sanitizes and limits extracted errors", func(t *testing.T) {
		t.Parallel()
		rule := Rule{JSON: &JSON{Pointer: "/ok", Equals: json.RawMessage(`true`), ErrorPointer: "/error"}}
		body, _ := json.Marshal(map[string]any{"ok": false, "error": "oops\n" + strings.Repeat("z", 1000)})
		err := rule.Evaluate(200, body)
		if err == nil || strings.Contains(err.Error(), "\n") || len(err.Error()) > 350 {
			t.Fatalf("unsafe error: %v", err)
		}
	})
}

func TestRuleClone(t *testing.T) {
	t.Parallel()

	original := Rule{
		StatusCodes: []int{200},
		Text:        &Text{Equals: str("ok")},
	}
	copy := original.Clone()
	original.StatusCodes[0] = 400
	*original.Text.Equals = "changed"
	if copy.StatusCodes[0] != 200 || *copy.Text.Equals != "ok" {
		t.Fatal("rule was not cloned")
	}
}

func TestHasBodyMatcher(t *testing.T) {
	t.Parallel()

	if (Rule{}).HasBodyMatcher() {
		t.Fatal("zero rule unexpectedly has a body matcher")
	}
	if !(Rule{Text: &Text{Equals: str("ok")}}).HasBodyMatcher() {
		t.Fatal("text matcher not detected")
	}
}

// TestRuleEvaluateCombined verifies status and body rules remain independent.
func TestRuleEvaluateCombined(t *testing.T) {
	t.Parallel()

	t.Run("accepts explicitly allowed status with matching text", func(t *testing.T) {
		t.Parallel()

		rule := Rule{
			StatusCodes: []int{418},
			Text: &Text{
				Equals: str("ok"),
			},
		}

		if err := rule.Evaluate(418, []byte("ok")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("accepts explicitly allowed status with matching JSON", func(t *testing.T) {
		t.Parallel()

		rule := Rule{
			StatusCodes: []int{418},
			JSON: &JSON{
				Pointer:      "/ok",
				Equals:       json.RawMessage(`true`),
				ErrorPointer: "/error",
			},
		}

		if err := rule.Evaluate(418, []byte(`{"ok":true,"error":"secret"}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejected status takes precedence over matching JSON", func(t *testing.T) {
		t.Parallel()

		rule := Rule{
			JSON: &JSON{
				Pointer:      "/ok",
				Equals:       json.RawMessage(`true`),
				ErrorPointer: "/error",
			},
		}

		err := rule.Evaluate(403, []byte(`{"ok":true,"error":"forbidden"}`))
		if err == nil || err.Error() != "webhook HTTP 403: unexpected HTTP status: forbidden" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed JSON never leaks rejected body", func(t *testing.T) {
		t.Parallel()

		rule := Rule{
			JSON: &JSON{
				Pointer:      "/ok",
				Equals:       json.RawMessage(`true`),
				ErrorPointer: "/error",
			},
		}

		err := rule.Evaluate(403, []byte(`{"token":"secret"} trailing`))
		if err == nil || err.Error() != "webhook HTTP 403: unexpected HTTP status" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("accepted status does not bypass text mismatch", func(t *testing.T) {
		t.Parallel()

		rule := Rule{
			StatusCodes: []int{200},
			Text: &Text{
				Equals: str("ok"),
			},
		}

		err := rule.Evaluate(200, []byte("failed\r\ninvalid auth"))
		if err == nil || err.Error() != "webhook HTTP 200: response did not match the acceptance rule: failed  invalid auth" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

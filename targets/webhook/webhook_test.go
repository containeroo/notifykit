package webhook

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNotification is a small notify.Notification implementation for tests.
type testNotification struct{}

// ID returns a static notification id.
func (testNotification) ID() string { return "n1" }

// Data returns webhook render data.
func (testNotification) Data(receiver string, vars map[string]any, title string) any {
	return map[string]any{
		"Receiver": receiver,
		"Title":    title,
		"Vars":     vars,
	}
}

// errReader always fails reads.
type errReader struct{}

// Read returns the configured read error.
func (errReader) Read(p []byte) (int, error) { return 0, errors.New("read failed") }

// TestNew tests expected behavior.
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults", func(t *testing.T) {
		t.Parallel()

		target := New()
		require.NotNil(t, target)
		assert.Equal(t, http.MethodPost, target.Method)
		assert.NotNil(t, target.Client)
		assert.NotNil(t, target.Logger)
		assert.Equal(t, LogResponseSummary, target.LogResponse)
		assert.Equal(t, 4096, target.ResponseBodyLimit)
	})

	t.Run("preserves custom dependencies", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{Timeout: time.Second}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		target := New(WithClient(client), WithLogger(logger))

		assert.True(t, target.Client == client)
		assert.True(t, target.Logger == logger)
	})
}

// TestTargetType tests expected behavior.
func TestTargetType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "webhook", (&Target{}).Type())
}

// TestTargetSend tests expected behavior.
func TestTargetSend(t *testing.T) {
	t.Parallel()

	t.Run("returns send result error", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.URL = "://bad-url"
		_, err := target.Send(context.Background(), payload())
		require.Error(t, err)
	})

	t.Run("returns header validation error", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.Headers = map[string]string{"": "value"}

		_, err := target.Send(context.Background(), payload())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook header name must not be empty")
	})

	t.Run("sends successfully", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		target := validTarget(t)
		target.URL = server.URL
		_, err := target.Send(context.Background(), payload())
		require.NoError(t, err)
	})
}

// TestTargetSendResult tests expected behavior.
func TestTargetSendResult(t *testing.T) {
	t.Parallel()

	t.Run("nil target errors", func(t *testing.T) {
		t.Parallel()

		var target *Target
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns render error", func(t *testing.T) {
		t.Parallel()

		target := &Target{}
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns header validation error", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.Headers = map[string]string{"X-Test": "ok\nInjected: yes"}

		result, err := target.SendResult(context.Background(), payload())

		require.Error(t, err)
		assert.Empty(t, result.Status)
		assert.Equal(t, 0, result.StatusCode)
		assert.Empty(t, result.Response)
		assert.Contains(t, err.Error(), "value must not contain newline characters")
	})

	t.Run("returns response details on success", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("response-body"))
		}))
		defer server.Close()

		target := validTarget(t)
		target.URL = server.URL
		target.ResponseBodyLimit = 8
		result, err := target.SendResult(context.Background(), payload())
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.Equal(t, "response", result.Response)
	})

	t.Run("returns response details on http error without leaking them through error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "secret-response-token", http.StatusBadGateway)
		}))
		defer server.Close()

		target := validTarget(t)
		target.URL = server.URL + "?token=secret-url-token"
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Equal(t, http.StatusBadGateway, result.StatusCode)
		assert.Contains(t, result.Response, "secret-response-token")
		assert.NotContains(t, err.Error(), "secret-response-token")
		assert.NotContains(t, err.Error(), "secret-url-token")
	})
}

// TestTargetValidate tests expected behavior.
func TestTargetValidate(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for renderable target", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		err := target.Validate(payload())
		require.NoError(t, err)
	})

	t.Run("returns render error", func(t *testing.T) {
		t.Parallel()

		target := &Target{}
		err := target.Validate(payload())
		require.Error(t, err)
	})

	t.Run("returns header validation error", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.Headers = map[string]string{" X-Test": "value"}

		err := target.Validate(payload())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not have leading or trailing whitespace")
	})
}

// TestTargetRender tests expected behavior.
func TestTargetRender(t *testing.T) {
	t.Parallel()

	t.Run("nil target errors", func(t *testing.T) {
		t.Parallel()

		var target *Target
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})

	t.Run("requires body template", func(t *testing.T) {
		t.Parallel()

		target := &Target{TitleTmpl: titleTemplate(t)}
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})

	t.Run("requires title template", func(t *testing.T) {
		t.Parallel()

		target := &Target{Template: bodyTemplate(t, `{"text":"ok"}`)}
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})

	t.Run("renders title into body", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		body, err := target.Render(payload())
		require.NoError(t, err)
		assert.JSONEq(t, `{"text":"title ops"}`, string(body))
	})

	t.Run("validates json when enabled", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.Template = bodyTemplate(t, `not-json`)
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})
}

// TestTargetPost tests expected behavior.
func TestTargetPost(t *testing.T) {
	t.Parallel()

	t.Run("posts body and headers", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "token", r.Header.Get("X-Token"))
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"ok":true}`, string(data))
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		target := New(WithURL(server.URL), WithHeader("X-Token", "token"))
		status, code, body, err := target.post(context.Background(), []byte(`{"ok":true}`))
		require.NoError(t, err)
		assert.Equal(t, "200 OK", status)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "ok", body)
	})

	t.Run("allows overriding content type", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/cloudevents+json", r.Header.Get("Content-Type"))
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		target := New(
			WithURL(server.URL),
			WithHeader("Content-Type", "application/cloudevents+json"),
		)
		_, _, _, err := target.post(context.Background(), []byte(`{"ok":true}`))
		require.NoError(t, err)
	})

	t.Run("returns header validation error", func(t *testing.T) {
		t.Parallel()

		target := New(
			WithURL("http://127.0.0.1/unused"),
			WithHeader("X-Test:Bad", "value"),
		)
		_, _, _, err := target.post(context.Background(), []byte("{}"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "contains invalid character")
	})

	t.Run("returns request creation error", func(t *testing.T) {
		t.Parallel()

		target := New(WithURL("://bad-url"))
		_, _, _, err := target.post(context.Background(), []byte("{}"))
		require.Error(t, err)
	})
}

// TestValidateHeaders tests expected behavior.
func TestValidateHeaders(t *testing.T) {
	t.Parallel()

	t.Run("accepts empty headers", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(nil)
		require.NoError(t, err)
	})

	t.Run("accepts custom headers", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{
			"X-Trace-ID": "",
			"X-Service":  "notifykit",
		})
		require.NoError(t, err)
	})

	t.Run("accepts content type override", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{
			"Content-Type": "application/cloudevents+json",
		})
		require.NoError(t, err)
	})

	t.Run("rejects empty custom header name", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook header name must not be empty")
	})

	t.Run("rejects custom header name with leading whitespace", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{" X-Test": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not have leading or trailing whitespace")
	})

	t.Run("rejects custom header name with trailing whitespace", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"X-Test ": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not have leading or trailing whitespace")
	})

	t.Run("rejects custom header name with colon", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"X-Test:Bad": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contains invalid character")
	})

	t.Run("rejects custom header name with non-ascii character", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"X-Ä": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contains invalid character")
	})

	t.Run("rejects custom header value with carriage return", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"X-Test": "ok\rInjected: yes"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must not contain newline characters")
	})

	t.Run("rejects custom header value with line feed", func(t *testing.T) {
		t.Parallel()

		err := validateHeaders(map[string]string{"X-Test": "ok\nInjected: yes"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must not contain newline characters")
	})
}

// TestTargetLogSuccessfulResponse tests expected behavior.
func TestTargetLogSuccessfulResponse(t *testing.T) {
	t.Parallel()

	resp := &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: http.Header{"X-Test": []string{"yes"}}}
	target := &Target{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	target.logSuccessfulResponse(resp, "body", false, time.Millisecond)
	target.LogResponse = LogResponseNone
	target.logSuccessfulResponse(resp, "body", false, time.Millisecond)
	target.LogResponse = LogResponseFull
	target.logSuccessfulResponse(resp, "body", false, time.Millisecond)
}

// TestTargetResponseLogFields tests expected behavior.
func TestTargetResponseLogFields(t *testing.T) {
	t.Parallel()

	resp := &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: http.Header{"X-Test": []string{"yes"}}}
	target := &Target{Name: "ops"}

	summary := target.responseLogFields(resp, "body", true, time.Millisecond, LogResponseSummary)
	assert.NotContains(t, summary, "responseBody")

	body := target.responseLogFields(resp, "body", true, time.Millisecond, LogResponseBody)
	assert.Contains(t, body, "responseBody")

	full := target.responseLogFields(resp, "body", true, time.Millisecond, LogResponseFull)
	assert.Contains(t, full, "responseHeaders")
}

// TestTargetLogFailedResponse tests expected behavior.
func TestTargetLogFailedResponse(t *testing.T) {
	t.Parallel()

	resp := &http.Response{Status: "502 Bad Gateway", StatusCode: http.StatusBadGateway, Header: http.Header{"X-Token": []string{"secret-header-token"}}}
	err := responseError("webhook", resp.Status)

	t.Run("uses summary fields by default", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		target := &Target{Logger: slog.New(slog.NewTextHandler(&logs, nil))}
		target.logFailedResponse(resp, "secret-response-token", false, time.Millisecond, err)

		assert.Contains(t, logs.String(), "webhook delivery failed")
		assert.NotContains(t, logs.String(), "secret-response-token")
		assert.NotContains(t, logs.String(), "secret-header-token")
	})

	t.Run("logs body only when explicitly configured", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		target := &Target{Logger: slog.New(slog.NewTextHandler(&logs, nil)), LogResponse: LogResponseBody}
		target.logFailedResponse(resp, "secret-response-token", false, time.Millisecond, err)

		assert.Contains(t, logs.String(), "secret-response-token")
		assert.NotContains(t, logs.String(), "secret-header-token")
	})
}

// TestTargetLabel tests expected behavior.
func TestTargetLabel(t *testing.T) {
	t.Parallel()

	t.Run("uses name", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "ops", (&Target{Name: "ops", URL: "http://example.com"}).label())
	})

	t.Run("uses secret-safe fallback", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "webhook", (&Target{URL: "http://example.com/secret"}).label())
	})
}

// TestNewClient tests expected behavior.
func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient(0, WithSkipTLSVerify(), WithProxyFromEnvironment())
	require.NotNil(t, client)
	assert.Equal(t, 10*time.Second, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.NotNil(t, transport.Proxy)
}

// TestReadResponseBody tests expected behavior.
func TestReadResponseBody(t *testing.T) {
	t.Parallel()

	t.Run("reads and trims response", func(t *testing.T) {
		t.Parallel()

		text, truncated, err := readResponseBody(strings.NewReader(" hello \n"), 10)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, "hello", text)
	})

	t.Run("truncates response", func(t *testing.T) {
		t.Parallel()

		text, truncated, err := readResponseBody(strings.NewReader("abcdef"), 3)
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Equal(t, "abc", text)
	})

	t.Run("returns read error", func(t *testing.T) {
		t.Parallel()

		text, truncated, err := readResponseBody(errReader{}, 3)
		require.Error(t, err)
		assert.False(t, truncated)
		assert.Empty(t, text)
	})
}

// TestResponseError tests expected behavior.
func TestResponseError(t *testing.T) {
	t.Parallel()

	err := responseError("ops", "500")
	assert.EqualError(t, err, `webhook "ops" delivery failed: 500`)
}

// TestTruncateBody tests expected behavior.
func TestTruncateBody(t *testing.T) {
	t.Parallel()

	t.Run("returns unchanged when limit disabled", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "abcdef", truncateBody("abcdef", 0))
	})

	t.Run("returns unchanged when below limit", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "abc", truncateBody("abc", 10))
	})

	t.Run("truncates above limit", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "abc...", truncateBody("abcdef", 3))
	})
}

// payload supports tests.
func payload() notify.Payload {
	return notify.Payload{Notification: testNotification{}, Receiver: "ops", Vars: map[string]any{"team": "platform"}}
}

// validTarget supports tests.
func validTarget(t *testing.T) *Target {
	t.Helper()
	return New(
		WithURL("http://127.0.0.1/unused"),
		WithTemplate(bodyTemplate(t, `{"text":{{ .Title | json }}}`)),
		WithTitleTemplate(titleTemplate(t)),
		WithValidateJSON(),
	)
}

// titleTemplate supports tests.
func titleTemplate(t *testing.T) *templates.StringTemplate {
	t.Helper()
	tmpl, err := templates.ParseStringTemplate("title", `title {{ .Receiver }}`)
	require.NoError(t, err)
	return tmpl
}

// bodyTemplate supports tests.
func bodyTemplate(t *testing.T, value string) *templates.Template {
	t.Helper()
	tmpl, err := templates.ParseTemplate("body", value, templates.WithDefaultFuncs())
	require.NoError(t, err)
	return tmpl
}

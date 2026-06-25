package webhook

import (
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

// ReceiverNames returns no receiver filter.
func (testNotification) ReceiverNames() []string { return nil }

// Data returns webhook render data.
func (testNotification) Data(receiver string, vars map[string]any, subject string) any {
	return map[string]any{
		"Receiver": receiver,
		"Subject":  subject,
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

	t.Run("returns response details on http error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad", http.StatusBadGateway)
		}))
		defer server.Close()

		target := validTarget(t)
		target.URL = server.URL
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Equal(t, http.StatusBadGateway, result.StatusCode)
		assert.Contains(t, result.Response, "bad")
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

		target := &Target{SubjectTmpl: subjectTemplate(t)}
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})

	t.Run("requires subject template", func(t *testing.T) {
		t.Parallel()

		target := &Target{Template: bodyTemplate(t, `{"text":"ok"}`)}
		body, err := target.Render(payload())
		require.Error(t, err)
		assert.Nil(t, body)
	})

	t.Run("renders subject into body", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		body, err := target.Render(payload())
		require.NoError(t, err)
		assert.JSONEq(t, `{"text":"subject ops"}`, string(body))
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

	t.Run("returns request creation error", func(t *testing.T) {
		t.Parallel()

		target := New(WithURL("://bad-url"))
		_, _, _, err := target.post(context.Background(), []byte("{}"))
		require.Error(t, err)
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

// TestTargetLabel tests expected behavior.
func TestTargetLabel(t *testing.T) {
	t.Parallel()

	t.Run("uses name", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "ops", (&Target{Name: "ops", URL: "http://example.com"}).label())
	})

	t.Run("falls back to url", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "http://example.com", (&Target{URL: "http://example.com"}).label())
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

	t.Run("without body", func(t *testing.T) {
		t.Parallel()

		err := responseError("ops", "500", "", false)
		assert.EqualError(t, err, `webhook "ops" delivery failed: 500`)
	})

	t.Run("with body", func(t *testing.T) {
		t.Parallel()

		err := responseError("ops", "500", "bad", false)
		assert.EqualError(t, err, `webhook "ops" delivery failed: 500: bad`)
	})

	t.Run("with truncated body", func(t *testing.T) {
		t.Parallel()

		err := responseError("ops", "500", "bad", true)
		assert.EqualError(t, err, `webhook "ops" delivery failed: 500: bad...`)
	})
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
		WithTemplate(bodyTemplate(t, `{"text":{{ .Subject | json }}}`)),
		WithSubjectTemplate(subjectTemplate(t)),
		WithValidateJSON(),
	)
}

// subjectTemplate supports tests.
func subjectTemplate(t *testing.T) *templates.StringTemplate {
	t.Helper()
	tmpl, err := templates.ParseStringTemplate("subject", `subject {{ .Receiver }}`)
	require.NoError(t, err)
	return tmpl
}

// bodyTemplate supports tests.
func bodyTemplate(t *testing.T, value string) *templates.Template {
	t.Helper()
	tmpl, err := templates.ParseTemplate("body", value)
	require.NoError(t, err)
	return tmpl
}

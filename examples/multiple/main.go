package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/targets/webhook"
	"github.com/containeroo/notifykit/templates"
)

const (
	ansiReset   = "\x1b[0m"
	ansiCyan    = "\x1b[36m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
)

// Alert is an application-owned event type that satisfies notify.Notification.
type Alert struct {
	IDValue string
	Service string
	Status  string
}

// ID returns a stable notification id for logs and delivery tracing.
func (a Alert) ID() string { return a.IDValue }

// Data builds the template context used by the title and webhook body.
func (a Alert) Data(receiver string, customData map[string]any, title string) any {
	return map[string]any{
		"ID":         a.IDValue,
		"Service":    a.Service,
		"Status":     a.Status,
		"Title":      title,
		"Receiver":   receiver,
		"CustomData": customData,
	}
}

// mockExternalAPI starts a local HTTP server and returns the configured status
// sequence one response at a time. Later attempts keep returning the final status.
func mockExternalAPI(name, color string, statuses ...int) (url string, stop func()) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}

		status := http.StatusOK
		if len(statuses) > 0 {
			status = statuses[min(attempt-1, len(statuses)-1)]
		}

		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s[%s]%s attempt %d -> %d %s | %s\n",
			color,
			name,
			ansiReset,
			attempt,
			status,
			http.StatusText(status),
			strings.TrimSpace(string(body)),
		)

		w.WriteHeader(status)
		_, _ = w.Write([]byte(http.StatusText(status)))
	}))

	return server.URL, server.Close
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	opsURL, stopOpsAPI := mockExternalAPI("ops", ansiCyan, http.StatusOK)
	defer stopOpsAPI()
	devURL, stopDevAPI := mockExternalAPI("dev", ansiYellow, http.StatusInternalServerError, http.StatusOK)
	defer stopDevAPI()
	auditURL, stopAuditAPI := mockExternalAPI("audit", ansiMagenta, http.StatusConflict, http.StatusOK)
	defer stopAuditAPI()

	title, err := templates.ParseStringTemplate("title", `{{ .Service }} is {{ .Status }}`)
	if err != nil {
		panic(err)
	}

	body, err := templates.ParseTemplate(
		"webhook",
		`{"text": {{ .Title | json }}, "receiver": {{ .Receiver | json }}, "team": {{ .CustomData.team | json }}, "environment": {{ .CustomData.environment | json }}}`,
		templates.WithDefaultFuncs(),
	)
	if err != nil {
		panic(err)
	}

	client := webhook.NewClient(5 * time.Second)

	// No RetryConfig means delivery is attempted exactly once.
	ops := notify.NewReceiver(
		"ops",
		webhook.New(
			webhook.WithName("mock-ops-api"),
			webhook.WithURL(opsURL),
			webhook.WithTitleTemplate(title),
			webhook.WithTemplate(body),
			webhook.WithClient(client),
			webhook.WithLogger(logger),
			webhook.WithValidateJSON(),
		),
	).WithCustomData(map[string]any{
		"team":        "operations",
		"environment": "production",
	})

	// DefaultRetryPolicy retries transient transport failures, 408, 429, and 5xx.
	dev := notify.NewReceiver(
		"dev",
		webhook.New(
			webhook.WithName("mock-dev-api"),
			webhook.WithURL(devURL),
			webhook.WithTitleTemplate(title),
			webhook.WithTemplate(body),
			webhook.WithClient(client),
			webhook.WithLogger(logger),
			webhook.WithValidateJSON(),
		),
	).WithCustomData(map[string]any{
		"team":        "developers",
		"environment": "development",
	}).WithRetry(notify.RetryConfig{
		Count:      3,
		Backoff:    time.Second,
		MaxBackoff: 10 * time.Second,
		Jitter:     true,
		Policy:     notify.DefaultRetryPolicy,
	})

	// Custom policies can opt into only the failures a receiver considers transient.
	audit := notify.NewReceiver(
		"audit",
		webhook.New(
			webhook.WithName("mock-audit-api"),
			webhook.WithURL(auditURL),
			webhook.WithTitleTemplate(title),
			webhook.WithTemplate(body),
			webhook.WithClient(client),
			webhook.WithLogger(logger),
			webhook.WithValidateJSON(),
		),
	).WithCustomData(map[string]any{
		"team":        "security",
		"environment": "production",
	}).WithRetry(notify.RetryConfig{
		Count:   2,
		Backoff: 500 * time.Millisecond,
		Policy: notify.AnyRetryPolicy(
			notify.RetryOnStatusCode(http.StatusConflict),
			notify.RetryOnServerError,
		),
	})

	err = notify.SendTo(ctx, Alert{
		IDValue: "alert-1",
		Service: "api",
		Status:  "down",
	}, ops, dev, audit)
	if err != nil {
		panic(err)
	}
}

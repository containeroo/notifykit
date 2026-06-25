package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/targets/webhook"
	"github.com/containeroo/notifykit/templates"
)

// Alert is an application-owned event type that satisfies notify.Notification.
type Alert struct {
	IDValue string
	Service string
	Status  string
}

// ID returns a stable notification id for logs and delivery tracing.
func (a Alert) ID() string { return a.IDValue }

// Data builds the template context used by the subject and webhook body.
func (a Alert) Data(receiver string, vars map[string]any, subject string) any {
	return map[string]any{
		"ID":       a.IDValue,
		"Service":  a.Service,
		"Status":   a.Status,
		"Subject":  subject,
		"Receiver": receiver,
		"Vars":     vars,
	}
}

// mockExternalAPI starts a local HTTP server that acts like an external webhook API.
func mockExternalAPI(name string) (url string, stop func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stdout, "mock external API %q received webhook notification\n", name) // nolint:errcheck

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	return server.URL, server.Close
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	opsURL, stopOpsAPI := mockExternalAPI("ops")
	defer stopOpsAPI()
	devURL, stopDevAPI := mockExternalAPI("dev")
	defer stopDevAPI()

	subject, err := templates.ParseStringTemplate("subject", `{{ .Service }} is {{ .Status }}`)
	if err != nil {
		panic(err)
	}

	body, err := templates.ParseTemplate("webhook", `{"text": {{ .Subject | json }}, "receiver": {{ .Receiver | json }}}`)
	if err != nil {
		panic(err)
	}

	client := webhook.NewClient(5 * time.Second)
	ops := notify.NewReceiver(
		"ops",
		webhook.New(
			webhook.WithName("mock-ops-api"),
			webhook.WithURL(opsURL),
			webhook.WithSubjectTemplate(subject),
			webhook.WithTemplate(body),
			webhook.WithClient(client),
			webhook.WithLogger(logger),
			webhook.WithValidateJSON(),
		),
	).WithVars(map[string]any{"team": "operations"})

	dev := notify.NewReceiver(
		"dev",
		webhook.New(
			webhook.WithName("mock-dev-api"),
			webhook.WithURL(devURL),
			webhook.WithSubjectTemplate(subject),
			webhook.WithTemplate(body),
			webhook.WithClient(client),
			webhook.WithLogger(logger),
			webhook.WithValidateJSON(),
		),
	).WithVars(map[string]any{"team": "developers"})

	err = notify.SendTo(ctx, Alert{
		IDValue: "alert-1",
		Service: "api",
		Status:  "down",
	}, ops, dev)
	if err != nil {
		panic(err)
	}
}

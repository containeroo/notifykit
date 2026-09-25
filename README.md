# Notifykit

A small Go library for templated webhook and email notifications.
Your application owns its events and configuration; Notifykit handles delivery.

## Features

- Synchronous delivery or a bounded queue with multiple workers.
- Receiver routing and per-receiver template data.
- Opt-in retries with backoff, jitter, and `Retry-After` support.
- HTTP webhooks and SMTP with explicit TLS modes.
- Text and HTML templates, structured errors, and graceful shutdown.

## Quick start

Requires Go 1.27 or newer:

```sh
go get github.com/containeroo/notifykit
```

Replace the example URL with your webhook endpoint:

```go
package main

import (
    "context"
    "log"
    "uuid"

    "github.com/containeroo/notifykit/notify"
    "github.com/containeroo/notifykit/targets/webhook"
    "github.com/containeroo/notifykit/templates"
)

type Alert struct {
    IDValue uuid.UUID
    Message string
}

func (a Alert) ID() uuid.UUID { return a.IDValue }
func (a Alert) Data(_ string, _ map[string]any, title string) any {
    return map[string]any{"Message": a.Message, "Title": title}
}

func main() {
    title, err := templates.ParseStringTemplate("title", "Service alert")
    if err != nil {
        log.Fatal(err)
    }
    body, err := templates.ParseTemplate(
        "body", `{"text": {{ printf "%s: %s" .Title .Message | json }}}`,
        templates.WithDefaultFuncs(),
    )
    if err != nil {
        log.Fatal(err)
    }
    target := webhook.New(
        webhook.WithURL("https://example.com/webhook"),
        webhook.WithTitleTemplate(title),
        webhook.WithTemplate(body),
        webhook.WithValidateJSON(),
    )
    if err := notify.SendTo(context.Background(), Alert{IDValue: uuid.NewV7(), Message: "API is down"},
        notify.NewReceiver("ops", target)); err != nil {
        log.Fatal(err)
    }
}
```

## Documentation

Start with the [documentation overview](docs/content/index.md) or [getting started](docs/content/getting-started.md). Full guides cover routing, retries, queueing, templates, SMTP, and logging/redaction.

Runnable examples: [single receiver](examples/single/main.go) and [multiple receivers](examples/multiple/main.go).

Documentation is built with [Lore](https://github.com/gi8lino/lore); see the [development guide](docs/content/development.md) for build instructions.

## License

[Apache License 2.0](LICENCE).

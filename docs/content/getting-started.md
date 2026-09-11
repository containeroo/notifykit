# Getting started

Install Notifykit with Go 1.24 or newer:

```sh
go get github.com/containeroo/notifykit
```

## Send a webhook

This complete program sends one notification synchronously. Replace the endpoint
URL before running it. The default HTTP client has a 10-second timeout; retries
are disabled unless you configure a policy.

```go
package main

import (
    "context"
    "log"

    "github.com/containeroo/notifykit/notify"
    "github.com/containeroo/notifykit/targets/webhook"
    "github.com/containeroo/notifykit/templates"
)

type Alert struct{ Message string }

func (a Alert) ID() string { return "alert-1" }
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
    if err := notify.SendTo(context.Background(), Alert{"API is down"},
        notify.NewReceiver("ops", target)); err != nil {
        log.Fatal(err)
    }
}
```

`ID` identifies the notification for logs and tracing. This example uses a fixed
ID; real applications should supply an identifier appropriate to each event.
`Data` builds the template context. The target calls it once to render the title,
then again with that title to render the body.

## Next steps

Use [receiver settings](receivers.md) for routing and custom data,
[retry policies](retries.md) for transient failures, or the
[manager](manager.md) for asynchronous delivery. For email, use the
[SMTP guide](email.md) with an [HTML template](templates.md).

## Runnable local examples

The repository examples start local HTTP test servers, so they do not need an
external webhook endpoint:

```sh
go run ./examples/single
go run ./examples/multiple
```

The multiple-receiver example demonstrates no retries, default transient retries,
and a custom policy for selected status codes.

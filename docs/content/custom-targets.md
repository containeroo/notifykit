# Custom targets

Implement `notify.Target` to deliver notifications through another transport:

```go
type Target interface {
    Send(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error)
    Type() string
}
```

`Type` returns a stable transport label such as `webhook` or `email`. A receiver
can combine built-in and custom targets:

```go
receiver := notify.NewReceiver("ops", webhookTarget, customTarget)
```

## Payload and rendering

`payload.ID()` returns the notification UUIDv7. `payload.Receiver` is the
receiver's display name, and `payload.CustomData` contains its custom template
data. `payload.Data(subject)` delegates to the application's notification.

Built-in targets first call `Data("")` to render a title or subject, then call it
again with the rendered value for the body. Follow the same pattern if your
transport uses templates.

## Results and errors

A nil error means delivery succeeded. Returning a non-success status code without
an error does not trigger retries. Populate `DeliveryResult` when your transport
has useful response details:

| Field        | Meaning                                                   |
| ------------ | --------------------------------------------------------- |
| `Status`     | Transport-specific status, for example `sent` or `failed` |
| `StatusCode` | HTTP-style status when applicable                         |
| `Response`   | Optional details; may be sensitive                        |
| `RetryAfter` | Minimum delay requested before another attempt            |

Mark invalid configuration or rendering failures with `notify.Permanent(err)`.
Mark network or stream failures with `notify.Transport(err)`. The built-in
policies give permanent classification precedence. Do not put unrelated protocol
codes into `StatusCode` and expect HTTP retry rules to apply.

Keep returned error messages safe for logs. Notifykit's webhook redactor is
specific to that target; it does not automatically sanitize custom-target errors.

See [retry policies](retries.md) and [logging and redaction](logging.md).

## Concurrency and cancellation

Targets may be called concurrently by multiple workers or application goroutines.
Keep configuration immutable and synchronize any mutable internal state. Honor
`ctx` throughout network operations so cancellation and shutdown can complete.

Notifykit invokes each target through its retry loop. If your client has its own
retry behavior, account for the combined attempt count and total wait. A shared
client or connection pool is useful when its implementation supports concurrent
use.

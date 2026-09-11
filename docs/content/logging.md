# Logging and redaction

Webhook URLs often contain credentials in their path or query string. Notifykit
hides those URLs in ordinary webhook error output while retaining the underlying
error for classification and retry decisions.

## What is redacted

A Go HTTP error can look like this:

```text
Post "https://example.com/SECRET?token=TOKEN": connection refused
```

Notifykit replaces its displayed message with:

```text
webhook URL or request failed
```

This applies to errors whose chain contains `*url.Error`, including URL parsing
and HTTP request failures. Response-body read failures are separately wrapped
with `webhook response read failed`. The wrapper is applied before the target
logs or returns the error, so ordinary manager and synchronous delivery error
logs also use the safe message.

This does not search arbitrary strings for tokens. Errors without a URL-error
cause are left unchanged by the URL redactor. Application-supplied target names,
receiver names, notification IDs, custom errors, and template errors are not a
place to put secrets.

## How the wrapper works

The internal wrapper provides a safe `Error()` string and an `Unwrap()` method
that returns the original cause:

```go
type redactedError struct {
    cause   error
    message string
}

func (e redactedError) Error() string { return e.message }
func (e redactedError) Unwrap() error { return e.cause }
```

Because the cause remains in the chain, `errors.Is`, `errors.As`, timeout
detection, and retry policies still work. For example:

```go
if errors.Is(err, context.DeadlineExceeded) {
    // The timeout remains detectable through the wrapper.
}
```

The wrapper is internal; applications should use standard error inspection and
Notifykit's classification helpers instead of depending on its concrete type.

## What remains sensitive

Unwrapping exposes the original error, including its URL:

```go
var original *url.Error
if errors.As(err, &original) {
    // original.URL contains the original endpoint. Do not log it directly.
}
```

Redaction controls the displayed error; it does not erase information from memory.
Custom loggers that unwrap errors or inspect internal structure can bypass it.
`DeliveryResult.Response` and `DeliveryError.Result.Response` can contain response
bodies with secrets. Notifykit's generic delivery logger does not print those
fields, and `DeliveryError.Error()` does not include the result body.

## Webhook response logging

Configure response logging with `webhook.WithLogResponse(...)`:

| Mode | Successful responses | Failed responses |
| --- | --- | --- |
| `LogResponseSummary` (default) | Status, code, duration, truncation | Same summary |
| `LogResponseNone` | No target response log | Summary still logged |
| `LogResponseBody` | Summary and response body | Summary and response body |
| `LogResponseFull` | Summary, body, and headers | Summary, body, and headers |

Body and full modes are explicit opt-ins. Response bodies and headers are not
scrubbed; only enable them when the destination's data is appropriate for your
logs. Response bodies are bounded by `WithResponseBodyLimit`, defaulting to 4096
bytes. That limit does not redact the content or bound response-header logging.

The configured target `Name` is used as its label; an unnamed target uses
`webhook`, never its URL. `LogResponseNone` only suppresses successful webhook
response logs: generic delivery logs and request/read errors can still appear.

## Logger ownership

The manager and `notify.Send` accept a logger for routing and delivery events.
Each webhook target has its own logger configured by `WithLogger`. Nil loggers
use discard handlers. `SendTo` uses a discard logger internally; it still returns
errors and preserves any logger explicitly attached to a webhook target.

See [webhook configuration](webhooks.md) and [retries and errors](retries.md).

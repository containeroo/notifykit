# Webhooks

Webhook targets render a title and request body, then send an HTTP request.

```go
client := webhook.NewClient(5*time.Second, webhook.WithProxyFromEnvironment())
target := webhook.New(
    webhook.WithName("ops-webhook"),
    webhook.WithURL("https://example.com/webhook"),
    webhook.WithMethod(http.MethodPost),
    webhook.WithHeader("Authorization", "Bearer <token>"),
    webhook.WithClient(client),
    webhook.WithTitleTemplate(title),
    webhook.WithTemplate(body),
    webhook.WithValidateJSON(),
)
```

Parse `title` with `templates.ParseStringTemplate` and `body` with
`templates.ParseTemplate`. Use `WithDefaultFuncs()` and the `json` helper to safely
encode dynamic JSON values. The [getting-started example](getting-started.md)
shows a complete program.

## Defaults and validation

| Setting | Default |
| --- | --- |
| Method | `POST` |
| Client timeout | 10 seconds |
| Request Content-Type | `application/json; charset=utf-8` |
| Response logging | Summary |
| Response body limit | 4096 bytes |
| Proxy environment variables | Disabled |

`WithHeaders` adds headers and can override Content-Type. Header names and newline
values are validated before delivery. The endpoint must be an absolute HTTP or
HTTPS URL. `WithValidateJSON` rejects invalid rendered JSON before sending.

`New` applies defaults but does not validate the templates or endpoint. Use
`target.Validate(payload)` to render and check configuration without sending, or
`target.Render(payload)` to inspect the body. `Send` validates before delivery.

## Clients and logging dependencies

Webhook targets own HTTP-specific dependencies. The manager keeps its own logger for queueing and dispatch logs, while each webhook target may receive a target-specific `Logger` and `Client`.

This keeps `notify.Manager` transport-agnostic and still lets applications provide a custom `*http.Client` for timeouts, transports, proxies, tracing, mTLS, or tests.

```go
client := webhook.NewClient(
    5*time.Second,
    webhook.WithProxyFromEnvironment(),
)
target := webhook.New(
    webhook.WithURL("https://example.com/webhook"),
    webhook.WithTitleTemplate(title),
    webhook.WithTemplate(body),
    webhook.WithClient(client),
    webhook.WithLogger(logger),
    webhook.WithValidateJSON(),
)
```

By default, `webhook.NewClient` does not use proxy environment variables. Add `webhook.WithProxyFromEnvironment()` when proxy support should be enabled. Add `webhook.WithSkipTLSVerify()` only for local development or trusted private endpoints with self-signed certificates.

## Responses

HTTP 2xx responses succeed. Other status codes return an error and populate
`DeliveryResult.StatusCode`; retry policy determines whether another attempt runs.
`Retry-After` supports seconds and HTTP dates. See [retries](retries.md).

`WithResponseBodyLimit` bounds captured response data. Nonpositive values use
4096 bytes. The reader consumes at most the limit plus one byte to detect
truncation. `DeliveryResult.Response` may contain sensitive data.

See [logging and redaction](logging.md) for `WithLogResponse`, safe error messages,
and details that remain accessible to callers.

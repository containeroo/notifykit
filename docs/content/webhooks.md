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

Parse `title` with `templates.ParseStringTemplate` and `body` with `templates.ParseTemplate`. Use `WithDefaultFuncs()` and the `json` helper to safely encode dynamic JSON values. The [getting-started example](getting-started.md) shows a complete program.

## Defaults and validation

| Setting                     | Default                           |
| --------------------------- | --------------------------------- |
| Method                      | `POST`                            |
| Client timeout              | 10 seconds                        |
| Request Content-Type        | `application/json; charset=utf-8` |
| Response logging            | Summary                           |
| Response body limit         | 4096 bytes                        |
| Proxy environment variables | Disabled                          |

`WithHeaders` adds headers and can override Content-Type. Header names and newline values are validated before delivery. The endpoint must be an absolute HTTP or HTTPS URL. `WithValidateJSON` rejects invalid rendered JSON before sending.

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

By default, HTTP 2xx responses succeed and non-2xx responses fail; **no response-body validation or error extraction is performed**. Existing targets remain unchanged. `DeliveryResult.StatusCode` is populated on either result. Retry policies determine whether failed deliveries are attempted again. `Retry-After` supports seconds and HTTP dates. See [retries](retries.md).

### Opt-in response validation

Enable response acceptance rules for a particular target using `WithResponseRule`:

```go
import (
    "encoding/json"

    "github.com/containeroo/notifykit/targets/webhook"
    "github.com/containeroo/notifykit/targets/webhook/response"
)

rule := response.Rule{
    JSON: &response.JSON{
        Pointer:      "/ok",
        Equals:       json.RawMessage(`true`),
        ErrorPointer: "/error",
    },
}
if err := rule.Validate(); err != nil {
    return err
}

target := webhook.New(
    webhook.WithURL("https://slack.com/api/chat.postMessage"),
    webhook.WithHeader("Authorization", "Bearer xoxb-REPLACE-ME"),
    webhook.WithTitleTemplate(title),
    webhook.WithTemplate(body),
    webhook.WithResponseRule(rule),
)
```

For example, Slack can return HTTP 200 with `{"ok":false,"error":"channel_not_found"}`. With this rule the delivery fails and its error reports `channel_not_found`. Without `WithResponseRule`, the same HTTP 200 continues to succeed.

`response.Rule` is independent of application configuration formats. Applications may parse status-code strings themselves, such as with HTTPKit's `parse.ParseStatusCodes`, and pass the resulting `[]int` as `StatusCodes`.

```go
rule := response.Rule{
    StatusCodes: []int{200, 201, 202},
    Text: &response.Text{
        Equals: &expectedText, // "ok", for example
    },
}
```

An omitted `StatusCodes` accepts every 2xx response. A non-empty list replaces that default, so even a non-2xx response can be accepted when explicitly listed. Rules support exactly one of `Text` and `JSON`:

- `Text.Equals` matches the complete response exactly (including whitespace).
- `Text.Contains` matches a non-empty substring.
- `JSON.Pointer` selects a nested field or array element using RFC 6901 JSON Pointer,
  such as `/result/delivery/accepted` or `/results/0/status`.
- `JSON.Equals` is an expected typed JSON value, such as `true`, `"sent"`, or `1`.
- `JSON.ErrorPointer` optionally selects a string, number, or boolean error value,
  such as `/error/details/message`, when validation fails.

Use `rule.Validate()` to validate rules ahead of delivery; the webhook target also rejects invalid rules before sending, marking them as permanent errors. Response evaluation is available independently through `rule.Evaluate(statusCode, body)`.

When a body matcher is enabled, responses larger than the configured body limit fail validation rather than accidentally passing against a truncated body. `WithResponseBodyLimit` defaults to 4096 bytes; increase it if your API returns larger successful JSON documents. Captured response data is still size-bounded. Only explicitly selected error values (or plain text with a text matcher) can appear in returned errors. They are stripped of control characters and limited to 256 bytes; do not select JSON fields containing credentials.

**Retry note:** The built-in `DefaultRetryPolicy` retries transport failures, 408, 429, and 5xx. A response-body mismatch on HTTP 200 returns a normal error but does not trigger a default retry. Choose `notify.RetryOnError` or a custom policy when you need retry behavior for such responses.

`WithResponseBodyLimit` bounds captured response data. Nonpositive values use 4096 bytes. The reader consumes at most the limit plus one byte to detect truncation. `DeliveryResult.Response` may contain sensitive data.

See [logging and redaction](logging.md) for `WithLogResponse`, safe error messages, and details that remain accessible to callers.

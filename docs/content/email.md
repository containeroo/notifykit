# Email

Email targets send an HTML message through SMTP. Use a contextually escaped HTML
body and a text subject:

```go
subject, err := templates.ParseStringTemplate("subject", "Alert: {{ .Service }}")
if err != nil {
    return err
}
body, err := templates.ParseHTMLTemplate("email", "<h1>{{ .Subject }}</h1><p>{{ .Message }}</p>")
if err != nil {
    return err
}
target := email.New(
    email.WithHost("smtp.example.com"),
    email.WithCredentials("user", "password"),
    email.WithFrom("alerts@example.com"),
    email.WithTo("ops@example.com"),
    email.WithCC("lead@example.com"),
    email.WithBCC("audit@example.com"),
    email.WithSubjectTemplate(subject),
    email.WithTemplate(body),
)
```

The notification's `Data` method must supply `Service` for the first render and
`Subject` and `Message` for the body render. See [templates](templates.md).

## Recipients and headers

`To` and `CC` are message headers. BCC recipients are included in the SMTP envelope
but never written to a BCC header. Pass each mailbox as a separate argument.

`WithHeader` and `WithHeaders` add custom headers. From, To, CC, BCC, Subject,
MIME-Version, and Content-Type are reserved. Custom header names and values are
validated, and headers are written in deterministic order.

## Encryption

| Mode | Behavior | Default port |
| --- | --- | --- |
| `email.TLSRequired` | Require STARTTLS before authentication or delivery | 587 |
| `email.TLSImplicit` | Start TLS before the SMTP greeting | 465 |
| `email.TLSPlaintext` | Explicitly use a plaintext relay | 587 |

`TLSRequired` is the default. Set a mode with `email.WithTLSMode(...)`. Explicit
ports override the mode's default. Plaintext mode rejects credentials. Certificate
verification is enabled; `WithSkipTLSVerify()` disables it only when you explicitly
choose to trust an otherwise unverifiable relay.

## Validation, retry classification, and timeouts

Rendered subjects reject CR and LF. Sender and recipient values must each be a
single bare mailbox such as `ops@example.com`; display-name forms and comma-separated
lists are rejected. Pass multiple recipients as separate `WithTo`/`WithCC`/`WithBCC`
arguments.

Email rendering and configuration errors are permanent. SMTP network failures,
timeouts, and temporary 4xx replies are retryable with `DefaultRetryPolicy`;
permanent 5xx replies and certificate verification failures are not. SMTP reply
codes are kept out of the HTTP-style `DeliveryResult.StatusCode`. Use `errors.As`
on the error for `*textproto.Error` when your application needs the SMTP code.

`email.WithTimeout(30*time.Second)` bounds each complete SMTP attempt, including
the greeting, TLS handshake, authentication, and message transfer. Nonpositive
values use the 30-second default. `WithDialTimeout` separately limits connection
establishment; the earlier caller deadline always wins. Retries have separate
attempt timeouts, so use a caller deadline to bound the entire operation.

`New` applies defaults; it does not connect or validate the full configuration.
`target.Validate(payload)` renders and validates without connecting.
`target.Render(payload)` returns the rendered subject/body. `Send` validates
before establishing the SMTP connection.

The body renderer accepts `templates.Renderer`. Existing text templates are
supported but do not automatically escape HTML. Prefer `ParseHTMLTemplate` or
`LoadHTMLSource` for notification data inserted into HTML.

# Retries and errors

Retries are explicit and opt-in. `RetryConfig.Policy` decides whether a failed delivery may be retried; when `Policy` is `nil`, Notifykit never retries, even when `Count` is greater than zero. `Count` is the number of retries after the initial attempt, so `Count: 2` allows at most three delivery attempts.

A typical configuration uses Notifykit's built-in transient-failure policy:

```go
receiver.WithRetry(notify.RetryConfig{
    Count:      3,
    Backoff:    time.Second,
    MaxBackoff: 30 * time.Second,
    Jitter:     true,
    Policy:     notify.DefaultRetryPolicy,
})
```

With this configuration, a failed delivery is first evaluated by the policy. If the policy returns `false`, the failure is returned immediately. If it returns `true` and attempts remain, Notifykit waits for the calculated retry delay and tries the target again.

## Retry configuration

| Option       | Meaning                                                                                                              |
| ------------ | -------------------------------------------------------------------------------------------------------------------- |
| `Count`      | Number of retries after the initial attempt. `0` means one attempt only.                                             |
| `Backoff`    | Delay before the first retry. Later local delays double exponentially. Zero or a negative value retries immediately. |
| `MaxBackoff` | Caps the calculated local exponential backoff. Zero or a negative value means no cap.                                |
| `Jitter`     | Applies full jitter to the local backoff, choosing a delay between zero and the calculated backoff.                  |
| `Policy`     | Function that decides whether a failed delivery is retryable. `nil` disables retries.                                |

## Built-in policies

Retry policies are small functions with this shape:

```go
type RetryPolicy func(result DeliveryResult, err error) bool
```

Notifykit provides reusable policies that can be used alone or combined:

| Policy                        | Retries when                                                                                                     |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `RetryOnTimeout`              | The error is classified as a transport failure and represents a timeout.                                         |
| `RetryOnNetworkError`         | The error is classified as a non-timeout transport failure.                                                      |
| `RetryOnStatusCode(codes...)` | `DeliveryResult.StatusCode` matches one of the supplied codes.                                                   |
| `RetryOnServerError`          | `DeliveryResult.StatusCode` is between 500 and 599.                                                              |
| `RetryOnError`                | Any non-permanent error is returned by the target. This matches Notifykit's original retry-every-error behavior. |
| `AnyRetryPolicy(...)`         | Any contained policy accepts the failure.                                                                        |

`DefaultRetryPolicy` is itself composed from the smaller policies:

```go
notify.AnyRetryPolicy(
    notify.RetryOnTimeout,
    notify.RetryOnNetworkError,
    notify.RetryOnStatusCode(408),
    notify.RetryOnStatusCode(429),
    notify.RetryOnServerError,
)
```

This means the default policy retries classified transport failures, HTTP-style `408 Request Timeout`, `429 Too Many Requests`, and status codes from `500` through `599`. It does not retry other failures.

To disable retries, leave the policy unset:

```go
receiver.WithRetry(notify.RetryConfig{})
```

To retry only selected status codes:

```go
receiver.WithRetry(notify.RetryConfig{
    Count:   5,
    Backoff: time.Second,
    Policy:  notify.RetryOnStatusCode(409, 425, 429),
})
```

To combine several built-in policies:

```go
receiver.WithRetry(notify.RetryConfig{
    Count:   4,
    Backoff: time.Second,
    Policy: notify.AnyRetryPolicy(
        notify.RetryOnStatusCode(409, 425, 429),
        notify.RetryOnServerError,
        notify.RetryOnTimeout,
        notify.RetryOnNetworkError,
    ),
})
```

A caller can also provide a completely custom policy:

```go
receiver.WithRetry(notify.RetryConfig{
    Count:   3,
    Backoff: time.Second,
    Policy: func(result notify.DeliveryResult, err error) bool {
        if notify.IsPermanent(err) {
            return false
        }
        return result.StatusCode == 409 || result.StatusCode == 429
    },
})
```

## Error classification

Notifykit separates failure classification from retry policy. Built-in targets classify failures so policies do not have to guess what an arbitrary `error` means:

```text
template / configuration error  -> notify.Permanent(err)
network / timeout / stream error -> notify.Transport(err)
HTTP response                    -> DeliveryResult.StatusCode
```

`notify.IsPermanent` and `notify.IsTransport` expose these classifications to custom policies. All built-in retry policies treat permanent failures as non-retryable. `RetryOnTimeout` and `RetryOnNetworkError` only operate on errors explicitly marked as transport failures, so an unrelated error cannot accidentally be interpreted as a network failure.

Custom targets should use the same classification when they can distinguish failure types. Custom policies may inspect the classifications and are free to define different behavior.

## Backoff, jitter, and `Retry-After`

Without jitter, local retry delays grow exponentially from `Backoff`. For example, `Backoff: 1s` produces `1s`, `2s`, `4s`, `8s`, and so on until `MaxBackoff` is reached. With `Jitter: true`, each local delay is randomized between zero and that calculated value to reduce synchronized retries across callers.

Targets may also return `DeliveryResult.RetryAfter` to request a minimum delay before the next attempt. The webhook target fills this automatically from the HTTP `Retry-After` response header and supports both delay-seconds and HTTP-date forms. The effective delay is:

```text
max(local backoff after jitter, DeliveryResult.RetryAfter)
```

A target-provided `RetryAfter` value is not capped by `MaxBackoff`, because it represents the receiver's minimum requested wait time. `RetryAfter` affects only the delay; the configured retry policy still decides whether another attempt is allowed.

## SMTP failures

Email classifies temporary SMTP 4xx replies as transport failures. Permanent SMTP
5xx replies, invalid configuration, rendering failures, and certificate
verification failures are permanent. SMTP codes are not placed in the HTTP-style
`DeliveryResult.StatusCode`; inspect `*textproto.Error` with `errors.As` if needed.
See [email delivery](email.md).

## Structured delivery errors

```go
var failure *notify.DeliveryError
if errors.As(err, &failure) {
    // ReceiverID, TargetType, TargetIndex, Attempts, Result, Err
}
```

`TargetIndex` is zero-based within the receiver's targets. `Send` joins multiple
failures with `errors.Join`; `errors.As` finds the first matching failure, while
`errors.Is` still finds underlying causes. Results can contain sensitive response
data and are not included in the delivery error string. See
[logging and redaction](logging.md).

## Cancellation and duplicate delivery

A canceled context stops retry attempts and interrupts backoff waits. Use a
caller deadline to bound the whole send, including retries across targets.
Targets within one notification are processed sequentially; a long retry wait
can delay its other destinations.

Retries can duplicate notifications if the receiver accepted a request but the
acknowledgement was lost. A notification ID is an application-owned string used for tracing; Notifykit
does not automatically deduplicate deliveries. Use endpoint-supported idempotency
when your application requires it.

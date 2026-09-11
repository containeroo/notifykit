# Receivers and routing

A receiver groups one or more delivery targets and optional receiver-scoped settings.

```go
receiver := notify.NewReceiver("ops", slackWebhook, emailTarget).
    WithName("Operations").
    WithCustomData(map[string]any{"channel": "alerts"}).
    WithRetry(notify.RetryConfig{Count: 2, Backoff: time.Second, Policy: notify.DefaultRetryPolicy})
```

`Receiver.ID` is the routing identifier. `Receiver.Name` is passed into the notification payload as the receiver name. When `ID` or `Name` is empty, Notifykit fills defaults from the receiver map key on internal copies during normalization.

## Notification contract

Applications implement this interface:

```go
type Notification interface {
    ID() string
    Data(receiver string, customData map[string]any, subject string) any
}
```

`ID` returns a stable notification identifier for logs and delivery tracing.

`Data` returns the template context. Targets call it twice: first with an empty string to render their title or subject template, then with the rendered value so the body template can reuse it. Webhook render data commonly exposes that value as `.Title`; email render data commonly exposes it as `.Subject`.

To select specific receivers, also implement `notify.ReceiverRouter`:

```go
type ReceiverRouter interface {
    ReceiverIDs() []ReceiverID
}
```

`ReceiverIDs` controls routing. The returned values are matched against the keys in the receiver map passed to `notify.Send`, `notify.SendTo`, or `notify.NewManager`.

```go
func (a Alert) ReceiverIDs() []notify.ReceiverID {
    return []notify.ReceiverID{"ops"}
}
```

Routing behavior:

```text
nil or empty ReceiverIDs()        send to all configured receivers
[]ReceiverID{"ops"}               send to receiver ID "ops"
[]ReceiverID{"ops", "dev"}        send to both receiver IDs
unknown receiver ID               skip that receiver and log a warning
```

Duplicate requested IDs are delivered once. Broadcast receivers are processed in ID order.
Receivers with no targets are rejected by `Send` and `NewManager`.

## Receiver helpers

`notify.NewReceiver` and the fluent receiver methods are convenience helpers for simple setup:

```go
receiver := notify.NewReceiver("ops", slackTarget, emailTarget).
    WithName("Operations").
    WithCustomData(map[string]any{"channel": "alerts"}).
    WithRetry(notify.RetryConfig{Count: 2, Backoff: time.Second, Policy: notify.DefaultRetryPolicy})

err := notify.SendTo(ctx, alert, receiver)
```

Available helpers:

```go
func NewReceiver(id ReceiverID, targets ...Target) *Receiver
func NewReceivers(receivers ...*Receiver) Receivers
func SendTo(ctx context.Context, notification Notification, receivers ...*Receiver) error

func (r *Receiver) WithName(name string) *Receiver
func (r *Receiver) WithCustomData(customData map[string]any) *Receiver
func (r *Receiver) WithRetry(cfg RetryConfig) *Receiver
func (r *Receiver) WithTargets(targets ...Target) *Receiver
```

`SendTo` is a small wrapper around `Send`: it builds a `Receivers` map from the provided receivers, uses a discard logger for Notifykit internals, resolves routing, and returns after delivery completes.

## Configuration maps

For config-driven applications, use a `notify.Receivers` map directly. The map key is the receiver ID used for routing.

```go
receivers := notify.Receivers{
    "ops": {
        Name: "Operations",
        Retry: notify.RetryConfig{
            Count:   2, // two retries, three total attempts
            Backoff: time.Second,
            Policy:  notify.DefaultRetryPolicy,
        },
        CustomData: map[string]any{
            "channel": "alerts",
        },
        Targets: []notify.Target{
            slackWebhook,
            emailTarget,
        },
    },
}

err := notify.Send(ctx, alert, receivers, logger)
```

## Defaults and ownership

An empty receiver ID or name defaults to the receiver **map key**. If you provide
an explicit ID, keep it consistent with the map key: routing always uses map keys.
Nil receiver entries are ignored. `NewReceivers` ignores nil arguments; if IDs
repeat, the later receiver wins. An empty ID is allowed as a map key.

Normalization copies receiver structs, target slices, and top-level `CustomData`
maps. `Manager.Receivers()` returns snapshots with the same copying rules.
Nested custom values and target objects remain shared and must not be mutated
during delivery. Each target receives its own top-level custom-data map.

Enqueued notifications must remain immutable until completion. `Data` methods
and custom targets must support concurrent calls when used concurrently.
`NewFromTarget` copies header maps and email recipient slices before applying
options; clients, loggers, templates, and other referenced objects remain shared.

See [custom targets](custom-targets.md) and [queued delivery](manager.md).

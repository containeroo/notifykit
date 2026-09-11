# Queued delivery

For queued asynchronous delivery, use `notify.NewManager`, start it once, and enqueue notifications over time.

```go
manager, err := notify.NewManager(receivers, logger)
if err != nil {
    panic(err)
}
if err := manager.Start(ctx); err != nil {
    panic(err)
}

queueID, err := manager.Enqueue(ctx, alert)
if err != nil {
    panic(err)
}
fmt.Println("queued notification", queueID)
```

Managers use one worker by default. Use `notify.WithWorkers` when queued notifications should be delivered concurrently:

```go
manager, err := notify.NewManager(receivers, logger, notify.WithWorkers(4))
```

When more than one worker is configured, different notifications may call the same target at the same time. Built-in targets are safe for this; custom targets should also be safe for concurrent `Send` calls.

## Capacity and completion

`notify.WithQueueCapacity(128)` bounds admitted queued notifications, excluding
active deliveries. Producers wait for admission before the manager stores their
notification; callers should use a context deadline to bound that wait. Blocked
caller goroutines still retain their own arguments, so applications should also
bound producer concurrency.

```go
manager, err := notify.NewManager(receivers, logger,
    notify.WithQueueCapacity(128),
    notify.WithWorkers(4),
    notify.WithOnComplete(func(result notify.Completion) {
        // QueueID, NotificationID, Err; nil Err means delivery succeeded.
        // Record metrics or an application-owned delivery outcome here.
    }),
)
```

Completion callbacks run on workers (or the cleanup goroutine for discarded work)
and must be concurrency-safe and return promptly. Do not enqueue into the same manager or wait for its shutdown from a
callback. Completion includes delivery failures and unresolved routing. The queue
is in-memory: callbacks do not make it durable across process crashes.

## Shutdown

Use `manager.Shutdown(ctx)` to stop accepting work and wait for accepted
notifications to finish. Keep the context passed to `Start` alive while draining.
If the shutdown deadline expires, active deliveries are canceled and remaining
queued work is discarded. Canceling the `Start` context also aborts delivery.

`manager.Wait(ctx)` waits for workers and completion callbacks to finish without
initiating shutdown. Use it after an aborted or timed-out shutdown if you need to
observe cleanup. Undispatched discarded notifications receive completion outcomes
with `notify.ErrManagerStopped`. Notifications already in delivery report their
delivery result or cancellation error. Enqueue after shutdown returns
`notify.ErrManagerStopped`; a manager cannot restart. Shutdown before Start
releases queued notifications as discarded work.

For a graceful shutdown, stop application producers, then drain with a separate
time budget while the context passed to `Start` is still alive:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := manager.Shutdown(shutdownCtx); err != nil {
    // The drain deadline expired; active work has been canceled.
    // Optionally call manager.Wait with another bounded context for cleanup.
    return err
}
```

`Shutdown` reports lifecycle completion, not individual target failures. Inspect
completion callbacks for delivery outcomes. A custom target must honor its
context; Notifykit cannot forcibly stop application code.

You may enqueue before `Start`, up to queue capacity. A successful `Enqueue`
means the notification was admitted, not delivered. Shutdown before Start discards
that work. The queue is not durable and has no automatic dead-letter storage.

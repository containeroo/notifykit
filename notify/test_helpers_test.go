package notify

import (
	"context"
	"io"
	"log/slog"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testNotification is a small Notification implementation for tests.
type testNotification struct {
	id        string
	receivers []ReceiverID
	dataFn    func(receiver string, vars map[string]any, subject string) any
}

// ID returns the configured notification id.
func (n testNotification) ID() string { return n.id }

// ReceiverIDs returns the configured receiver IDs.
func (n testNotification) ReceiverIDs() []ReceiverID { return n.receivers }

// Data returns configured render data or a default map.
func (n testNotification) Data(receiver string, vars map[string]any, subject string) any {
	if n.dataFn != nil {
		return n.dataFn(receiver, vars, subject)
	}
	return map[string]any{"receiver": receiver, "vars": vars, "subject": subject}
}

// testTarget records Send calls.
type testTarget struct {
	targetType string
	result     DeliveryResult
	err        error
	calls      int
	payload    Payload
}

// Send records the payload and returns the configured delivery details.
func (t *testTarget) Send(ctx context.Context, payload Payload) (DeliveryResult, error) {
	t.calls++
	t.payload = payload
	return t.result, t.err
}

// Type returns the configured target type.
func (t *testTarget) Type() string {
	if t.targetType == "" {
		return "test"
	}
	return t.targetType
}

// testResultTarget records SendResult calls.
type testResultTarget struct {
	testTarget
	result DeliveryResult
}

// Send records the payload and returns configured delivery details.
func (t *testResultTarget) Send(ctx context.Context, payload Payload) (DeliveryResult, error) {
	t.calls++
	t.payload = payload
	return t.result, t.err
}

// testDelivery records Dispatch calls.
type testDelivery struct {
	err       error
	calls     int
	payload   Payload
	receivers []*Receiver
}

// Dispatch records the payload and returns the configured error.
func (d *testDelivery) Dispatch(ctx context.Context, payload Payload, receivers []*Receiver) error {
	d.calls++
	d.payload = payload
	d.receivers = receivers
	return d.err
}

package notify

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"uuid"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testNotification is a small Notification implementation for tests.
type testNotification struct {
	id        string
	receivers []ReceiverID
	dataFn    func(receiver string, customData map[string]any, subject string) any
}

// ID returns a deterministic UUIDv7 for the configured test label.
func (n testNotification) ID() uuid.UUID {
	if n.id == "" {
		return uuid.Nil()
	}

	sum := sha256.Sum256([]byte(n.id))
	var id uuid.UUID
	copy(id[:], sum[:16])
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id
}

// ReceiverIDs returns the configured receiver IDs.
func (n testNotification) ReceiverIDs() []ReceiverID { return n.receivers }

// Data returns configured render data or a default map.
func (n testNotification) Data(receiver string, customData map[string]any, subject string) any {
	if n.dataFn != nil {
		return n.dataFn(receiver, customData, subject)
	}
	return map[string]any{"receiver": receiver, "CustomData": customData, "subject": subject}
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

// testResultTarget records Send calls with delivery details.
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

// testDelivery records dispatch calls.
type testDelivery struct {
	err       error
	calls     int
	payload   Payload
	receivers []*Receiver
}

// dispatch records the payload and returns the configured error.
func (d *testDelivery) dispatch(ctx context.Context, payload Payload, receivers []*Receiver) error {
	d.calls++
	d.payload = payload
	d.receivers = receivers
	return d.err
}

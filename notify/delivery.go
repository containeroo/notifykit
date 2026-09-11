package notify

import (
	"context"
	"errors"
	"log/slog"
	"maps"
)

// deliveryEngine sends notifications to runtime targets.
type deliveryEngine struct {
	logger *slog.Logger
}

// newDelivery constructs the default receiver delivery engine.
func newDelivery(logger *slog.Logger) *deliveryEngine {
	return &deliveryEngine{logger: logger}
}

// dispatch sends a notification payload to each receiver target.
func (d *deliveryEngine) dispatch(ctx context.Context, payload Payload, receivers []*Receiver) error {
	var errs []error
	for _, receiver := range receivers {
		if err := d.dispatchReceiver(ctx, receiver, payload); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// dispatchReceiver sends a payload to every target configured on one receiver.
func (d *deliveryEngine) dispatchReceiver(ctx context.Context, receiver *Receiver, payload Payload) error {
	var errs []error
	for index, target := range receiver.Targets {
		targetPayload := payload
		targetPayload.Receiver = receiver.Name
		targetPayload.CustomData = maps.Clone(receiver.CustomData)

		result, attempts, err := withRetry(ctx, d.logger, receiver.Retry, func() (DeliveryResult, error) {
			return target.Send(ctx, targetPayload)
		})
		if err != nil {
			errs = append(errs, &DeliveryError{ReceiverID: receiver.ID, TargetType: target.Type(), TargetIndex: index, Attempts: attempts, Result: result, Err: err})
			d.logger.Error(
				"notification target failed",
				"receiver", receiver.Name,
				"targetType", target.Type(),
				"notificationID", payload.ID(),
				"attempts", attempts,
				"status", result.Status,
				"statusCode", result.StatusCode,
				"error", err,
			)
			continue
		}

		d.logger.Info(
			"notification target delivered",
			"receiver", receiver.Name,
			"targetType", target.Type(),
			"notificationID", payload.ID(),
			"attempts", attempts,
			"status", result.Status,
			"statusCode", result.StatusCode,
		)
	}
	return errors.Join(errs...)
}

package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// deliveryEngine sends notifications to runtime targets.
type deliveryEngine struct {
	logger *slog.Logger
}

// NewDelivery constructs the default receiver delivery engine.
func NewDelivery(logger *slog.Logger) Delivery {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &deliveryEngine{logger: logger}
}

// Dispatch sends a notification payload to each receiver target.
func (d *deliveryEngine) Dispatch(ctx context.Context, payload Payload, receivers []*Receiver) error {
	if d == nil {
		return errors.New("delivery is nil")
	}

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
	if receiver == nil {
		return errors.New("receiver is nil")
	}

	var errs []error
	for _, target := range receiver.Targets {
		if target == nil {
			errs = append(errs, errors.New("target is nil"))
			continue
		}

		targetPayload := payload
		targetPayload.Receiver = receiver.Name
		targetPayload.Vars = receiver.Vars

		result, attempts, err := withRetry(ctx, d.logger, receiver.Retry, func() (DeliveryResult, error) {
			return target.Send(ctx, targetPayload)
		})
		if err != nil {
			errs = append(errs, err)
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

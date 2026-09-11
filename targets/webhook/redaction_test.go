package webhook

import (
	"bytes"
	"context"
	"errors"
	"github.com/containeroo/notifykit/notify"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
)

func TestTransportURLRedaction(t *testing.T) {
	for _, cause := range []error{errors.New("connection refused"), context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			target := validTarget(t)
			target.URL = "https://example.com/SECRET?token=TOKEN"
			target.Logger = logger
			target.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })}
			err := notify.Send(t.Context(), testNotification{}, notify.NewReceivers(notify.NewReceiver("ops", target)), logger)
			require.ErrorIs(t, err, cause)
			var original *url.Error
			require.ErrorAs(t, err, &original)
			require.Equal(t, target.URL, original.URL)
			require.True(t, notify.IsTransport(err))
			require.Equal(t, errors.Is(cause, context.DeadlineExceeded), notify.RetryOnTimeout(notify.DeliveryResult{}, err))
			for _, secret := range []string{"SECRET", "TOKEN"} {
				require.NotContains(t, err.Error(), secret)
				require.NotContains(t, logs.String(), secret)
			}
		})
	}
}
func TestInvalidURLRedaction(t *testing.T) {
	target := validTarget(t)
	// Invalid URL escapes in a path are rejected by the URL parser.
	target.URL = "https://example.com/SECRET%zz?token=TOKEN"
	for _, err := range []error{target.Validate(payload())} {
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET")
		require.NotContains(t, err.Error(), "TOKEN")
	}
	_, err := target.Send(t.Context(), payload())
	require.True(t, notify.IsPermanent(err))
	require.NotContains(t, err.Error(), "SECRET")
}

package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http/httptest"
	"net/textproto"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubjectAndAddressValidation(t *testing.T) {
	for _, subject := range []string{"hello\r\nX-Injected: yes", "hello\n\nbody", "hello\rX: yes"} {
		target := validTarget(t)
		target.SubjectTmpl, _ = templates.ParseStringTemplate("subject", subject)
		require.Error(t, target.Validate(payload()))
		_, err := target.Send(t.Context(), payload())
		require.True(t, notify.IsPermanent(err))
		require.False(t, notify.RetryOnError(notify.DeliveryResult{}, err))
	}
	for _, address := range []string{"a@example.com\r\nX: yes", "", "a@example.com,b@example.com"} {
		target := validTarget(t)
		target.To = []string{address}
		require.Error(t, target.Validate(payload()))
	}
}
func TestSMTPErrorClassification(t *testing.T) {
	target := validTarget(t)
	result, err := target.Send(t.Context(), payload())
	require.True(t, notify.IsTransport(err))
	require.True(t, notify.DefaultRetryPolicy(result, err))
	target.Host = ""
	result, err = target.Send(t.Context(), payload())
	require.True(t, notify.IsPermanent(err))
	require.False(t, notify.RetryOnError(result, err))
	for _, code := range []int{421, 450, 451, 550, 535} {
		cause := &textproto.Error{Code: code, Msg: "test"}
		err := classifySMTPError(cause)
		require.ErrorIs(t, err, cause)
		require.Equal(t, code < 500, notify.DefaultRetryPolicy(notify.DeliveryResult{}, err))
	}
}

// runScriptSMTP provides bounded local sessions and waits for all connections to close.
func runScriptSMTP(t *testing.T, script func(net.Conn)) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			script(conn)
			conn.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	return "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
}
func TestSMTPRepliesDriveRetries(t *testing.T) {
	for _, code := range []int{421, 550} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls atomic.Int32
			host, port := runScriptSMTP(t, func(conn net.Conn) { calls.Add(1); fmt.Fprintf(conn, "%d unavailable\r\n", code) })
			target := validTarget(t)
			target.Host = host
			target.Port = port
			err := notify.SendTo(t.Context(), testNotification{}, notify.NewReceiver("ops", target).WithRetry(notify.RetryConfig{Count: 2, Policy: notify.DefaultRetryPolicy}))
			require.Error(t, err)
			expected := int32(1)
			if code == 421 {
				expected = 3
			}
			require.Equal(t, expected, calls.Load())
		})
	}
}
func TestSMTPAttemptTimeout(t *testing.T) {
	for _, stage := range []string{"greeting", "ehlo", "data", "starttls", "implicit-tls"} {
		t.Run(stage, func(t *testing.T) {
			host, port := runScriptSMTP(t, func(conn net.Conn) {
				reader := bufio.NewReader(conn)
				writer := bufio.NewWriter(conn)
				if stage == "greeting" || stage == "implicit-tls" {
					io.Copy(io.Discard, conn)
					return
				}
				writeSMTP(writer, "220 localhost ESMTP")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if stage == "ehlo" {
						reader.ReadString('\n')
						return
					}
					if stage == "starttls" {
						writeSMTP(writer, "250-localhost")
						writeSMTP(writer, "250 STARTTLS")
						reader.ReadString('\n')
						writeSMTP(writer, "220 Ready for TLS")
						io.Copy(io.Discard, reader)
						return
					}
					if line == "DATA\r\n" {
						reader.ReadString('\n')
						return
					}
					writeSMTP(writer, "250 OK")
				}
			})
			target := validTarget(t)
			target.Host = host
			target.Port = port
			target.Timeout = 40 * time.Millisecond
			if stage == "implicit-tls" {
				target.TLSMode = TLSImplicit
			} else if stage == "starttls" {
				target.TLSMode = TLSRequired
			}
			started := time.Now()
			result, err := target.Send(t.Context(), payload())
			require.Error(t, err)
			require.True(t, notify.RetryOnTimeout(result, err))
			require.Less(t, time.Since(started), time.Second)
		})
	}
}
func TestSMTPCallerCancellation(t *testing.T) {
	entered := make(chan struct{})
	host, port := runScriptSMTP(t, func(conn net.Conn) { close(entered); bufio.NewReader(conn).ReadString('\n') })
	target := validTarget(t)
	target.Host = host
	target.Port = port
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := target.Send(ctx, payload()); result <- err }()
	<-entered
	cancel()
	select {
	case err := <-result:
		require.True(t, errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("SMTP cancellation stalled")
	}
}

func TestSMTPCertificateFailureIsPermanent(t *testing.T) {
	certServer := httptest.NewTLSServer(nil)
	config := certServer.TLS.Clone()
	certServer.Close()
	host, port := runScriptSMTP(t, func(conn net.Conn) { tls.Server(conn, config).Handshake() })
	target := validTarget(t)
	target.Host, target.Port, target.TLSMode = host, port, TLSImplicit
	result, err := target.Send(t.Context(), payload())
	require.Error(t, err)
	require.True(t, notify.IsPermanent(err))
	require.False(t, notify.DefaultRetryPolicy(result, err))
}

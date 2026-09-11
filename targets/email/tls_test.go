package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTLSModes(t *testing.T) {
	require.Equal(t, TLSRequired, New().TLSMode)
	require.Equal(t, 465, New(WithTLSMode(TLSImplicit)).Port)
	for _, mode := range []TLSMode{TLSRequired, TLSPlaintext} {
		t.Run(string(mode), func(t *testing.T) {
			host, port, _, stop := startSMTPServer(t)
			defer stop()
			target := validTarget(t)
			target.Host = host
			target.Port = port
			target.TLSMode = mode
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err := target.Send(ctx, payload())
			if mode == TLSRequired {
				require.ErrorContains(t, err, "required STARTTLS")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestEncryptedSMTP(t *testing.T) {
	certServer := httptest.NewTLSServer(nil)
	config := certServer.TLS.Clone()
	certServer.Close()
	for _, mode := range []TLSMode{TLSRequired, TLSImplicit} {
		t.Run(string(mode), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			done := make(chan struct{})
			messages := make(chan string, 1)
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				if mode == TLSRequired {
					reader := bufio.NewReader(conn)
					writer := bufio.NewWriter(conn)
					writeSMTP(writer, "220 localhost ESMTP")
					reader.ReadString('\n')
					writeSMTP(writer, "250-localhost")
					writeSMTP(writer, "250 STARTTLS")
					reader.ReadString('\n')
					writeSMTP(writer, "220 Ready for TLS")
				}
				secured := tls.Server(conn, config)
				if secured.Handshake() != nil {
					return
				}
				if mode == TLSImplicit {
					handleSMTPConn(secured, messages)
				} else {
					handleSMTPAfterTLS(secured, messages)
				}
			}()
			target := validTarget(t)
			target.Host = "127.0.0.1"
			target.Port = listener.Addr().(*net.TCPAddr).Port
			target.TLSMode = mode
			target.SkipTLSVerify = true
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			_, err = target.Send(ctx, payload())
			require.NoError(t, err)
			select {
			case msg := <-messages:
				require.Contains(t, msg, "Subject:")
			case <-ctx.Done():
				t.Fatal("no SMTP message")
			}
			<-done
		})
	}
}

// Adapt the existing SMTP handler without sending a second greeting after STARTTLS.
func handleSMTPAfterTLS(conn net.Conn, messages chan<- string) {
	proxyClient, proxyServer := net.Pipe()
	defer proxyClient.Close()
	go handleSMTPConn(proxyServer, messages)
	reader := bufio.NewReader(proxyClient)
	reader.ReadString('\n')
	done := make(chan struct{})
	go func() { defer close(done); io.Copy(conn, reader) }()
	io.Copy(proxyClient, conn)
	proxyClient.Close()
	<-done
}

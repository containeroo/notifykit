package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/notifykit/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithProxyFromEnvironment verifies proxy environment support is opt-in.
func TestWithProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	target := New(WithProxyFromEnvironment())
	assert.True(t, target.ProxyFromEnvironment)
}

// TestProxyDialAddress verifies default and explicit proxy ports.
func TestProxyDialAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		proxyURL string
		expected string
	}{
		{name: "http default", proxyURL: "http://proxy.example.com", expected: "proxy.example.com:80"},
		{name: "https default", proxyURL: "https://proxy.example.com", expected: "proxy.example.com:443"},
		{name: "explicit", proxyURL: "http://proxy.example.com:3128", expected: "proxy.example.com:3128"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			proxy, err := url.Parse(test.proxyURL)
			require.NoError(t, err)
			address, err := proxyDialAddress(proxy)
			require.NoError(t, err)
			assert.Equal(t, test.expected, address)
		})
	}
}

// TestDialProxy verifies CONNECT tunneling and proxy Basic authentication.
func TestDialProxy(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close() // nolint:errcheck

	requests := make(chan string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close() // nolint:errcheck

		reader := bufio.NewReader(conn)
		var request strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			request.WriteString(line)
			if line == "\r\n" {
				break
			}
		}
		requests <- request.String()
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	}()

	proxy, err := url.Parse("http://user:secret@" + listener.Addr().String())
	require.NoError(t, err)
	conn, err := dialProxy(context.Background(), proxy, "smtp.example.com:465", time.Second)
	require.NoError(t, err)
	defer conn.Close() // nolint:errcheck

	request := <-requests
	assert.Contains(t, request, "CONNECT smtp.example.com:465 HTTP/1.1")
	assert.Contains(t, request, "Proxy-Authorization: Basic dXNlcjpzZWNyZXQ=")
	assert.NotContains(t, request, "user:secret")
}

// TestProxyErrorClassification verifies transient proxy failures follow retry policy.
func TestProxyErrorClassification(t *testing.T) {
	t.Parallel()

	t.Run("gateway failure is transport", func(t *testing.T) {
		t.Parallel()
		err := classifySMTPError(&ProxyError{StatusCode: 502, Status: "502 Bad Gateway"})
		assert.True(t, notify.IsTransport(err))
	})

	t.Run("authentication failure is permanent", func(t *testing.T) {
		t.Parallel()
		err := classifySMTPError(&ProxyError{StatusCode: 407, Status: "407 Proxy Authentication Required"})
		assert.True(t, notify.IsPermanent(err))
	})
}

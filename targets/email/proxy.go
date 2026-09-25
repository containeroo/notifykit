package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyError reports a rejected HTTP CONNECT tunnel without exposing credentials.
type ProxyError struct {
	// StatusCode is the HTTP response status returned by the proxy.
	StatusCode int
	// Status is the HTTP status text returned by the proxy.
	Status string
}

// Error returns the proxy CONNECT failure.
func (e *ProxyError) Error() string {
	if e == nil {
		return "SMTP proxy CONNECT failed"
	}
	return "SMTP proxy CONNECT failed: " + e.Status
}

// resolveProxy resolves the standard proxy environment for the SMTP target.
func resolveProxy(address string, mode TLSMode) (*url.URL, error) {
	schemes := []string{"http", "https"}
	if mode == TLSImplicit {
		schemes[0], schemes[1] = schemes[1], schemes[0]
	}

	for _, scheme := range schemes {
		request := &http.Request{URL: &url.URL{Scheme: scheme, Host: address}}
		proxy, err := http.ProxyFromEnvironment(request)
		if err != nil {
			return nil, err
		}
		if proxy != nil {
			return proxy, nil
		}
	}
	return nil, nil
}

// dialProxy opens an HTTP CONNECT tunnel to the SMTP server.
func dialProxy(ctx context.Context, proxy *url.URL, target string, timeout time.Duration) (net.Conn, error) {
	if proxy == nil {
		return nil, errors.New("proxy URL is nil")
	}
	if proxy.Host == "" {
		return nil, errors.New("proxy host is empty")
	}

	proxyAddress, err := proxyDialAddress(proxy)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("connect proxy %s: %w", proxy.Host, err)
	}

	if strings.EqualFold(proxy.Scheme, "https") {
		host := proxy.Hostname()
		secured := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("TLS handshake with proxy %s: %w", proxy.Host, err)
		}
		conn = secured
	} else if proxy.Scheme != "" && !strings.EqualFold(proxy.Scheme, "http") {
		_ = conn.Close()
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxy.Scheme)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	if err := writeConnectRequest(conn, proxy, target); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	request := &http.Request{Method: http.MethodConnect}
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read proxy CONNECT response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		_ = conn.Close()
		return nil, &ProxyError{StatusCode: response.StatusCode, Status: response.Status}
	}

	_ = conn.SetDeadline(time.Time{})
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

// proxyDialAddress adds the standard port when a proxy URL omits one.
func proxyDialAddress(proxy *url.URL) (string, error) {
	if proxy == nil || proxy.Hostname() == "" {
		return "", errors.New("proxy host is empty")
	}
	if proxy.Port() != "" {
		return proxy.Host, nil
	}

	port := "80"
	if strings.EqualFold(proxy.Scheme, "https") {
		port = "443"
	} else if proxy.Scheme != "" && !strings.EqualFold(proxy.Scheme, "http") {
		return "", fmt.Errorf("unsupported proxy scheme %q", proxy.Scheme)
	}
	return net.JoinHostPort(proxy.Hostname(), port), nil
}

// writeConnectRequest sends an HTTP CONNECT request without exposing proxy credentials in errors.
func writeConnectRequest(conn net.Conn, proxy *url.URL, target string) error {
	var request strings.Builder
	fmt.Fprintf(&request, "CONNECT %s HTTP/1.1\r\n", target)
	fmt.Fprintf(&request, "Host: %s\r\n", target)
	fmt.Fprint(&request, "Proxy-Connection: Keep-Alive\r\n")
	if proxy.User != nil {
		username := proxy.User.Username()
		password, _ := proxy.User.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		fmt.Fprintf(&request, "Proxy-Authorization: Basic %s\r\n", credentials)
	}
	fmt.Fprint(&request, "\r\n")

	if _, err := conn.Write([]byte(request.String())); err != nil {
		return fmt.Errorf("write proxy CONNECT request: %w", err)
	}
	return nil
}

// bufferedConn preserves bytes already read while parsing the CONNECT response.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read consumes buffered tunnel bytes before reading from the underlying connection.
func (c *bufferedConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

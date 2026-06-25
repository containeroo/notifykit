package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNotification is a small notify.Notification implementation for tests.
type testNotification struct{}

// ID returns a static notification id.
func (testNotification) ID() string { return "n1" }

// ReceiverNames returns no receiver filter.
func (testNotification) ReceiverNames() []string { return nil }

// Data returns email render data.
func (testNotification) Data(receiver string, vars map[string]any, subject string) any {
	return map[string]any{
		"Receiver": receiver,
		"Subject":  subject,
		"Vars":     vars,
	}
}

// TestNew tests expected behavior.
func TestNew(t *testing.T) {
	t.Parallel()

	target := New()
	require.NotNil(t, target)
	assert.Equal(t, 587, target.Port)
}

// TestTargetType tests expected behavior.
func TestTargetType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "email", (&Target{}).Type())
}

// TestTargetSend tests expected behavior.
func TestTargetSend(t *testing.T) {
	t.Parallel()

	t.Run("returns send error", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		target.Host = "127.0.0.1"
		target.Port = 1
		_, err := target.Send(context.Background(), payload())
		require.Error(t, err)
	})

	t.Run("sends successfully", func(t *testing.T) {
		t.Parallel()

		host, port, messages, stop := startSMTPServer(t)
		defer stop()

		target := validTarget(t)
		target.Host = host
		target.Port = port
		_, err := target.Send(context.Background(), payload())
		require.NoError(t, err)
		assert.Contains(t, <-messages, "hello subject ops")
	})
}

// TestTargetSendResult tests expected behavior.
func TestTargetSendResult(t *testing.T) {
	t.Parallel()

	t.Run("nil target errors", func(t *testing.T) {
		t.Parallel()

		var target *Target
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns failed status on render error", func(t *testing.T) {
		t.Parallel()

		target := &Target{}
		result, err := target.SendResult(context.Background(), payload())
		require.Error(t, err)
		assert.Equal(t, "failed", result.Status)
	})

	t.Run("returns sent status", func(t *testing.T) {
		t.Parallel()

		host, port, _, stop := startSMTPServer(t)
		defer stop()

		target := validTarget(t)
		target.Host = host
		target.Port = port
		result, err := target.SendResult(context.Background(), payload())
		require.NoError(t, err)
		assert.Equal(t, "sent", result.Status)
		assert.NotEmpty(t, result.Response)
	})
}

// TestTargetValidate tests expected behavior.
func TestTargetValidate(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for renderable target", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		err := target.Validate(payload())
		require.NoError(t, err)
	})

	t.Run("returns render error", func(t *testing.T) {
		t.Parallel()

		target := &Target{}
		err := target.Validate(payload())
		require.Error(t, err)
	})
}

// TestTargetRender tests expected behavior.
func TestTargetRender(t *testing.T) {
	t.Parallel()

	t.Run("nil target errors", func(t *testing.T) {
		t.Parallel()

		var target *Target
		message, err := target.Render(payload())
		require.Error(t, err)
		assert.Empty(t, message)
	})

	t.Run("requires body template", func(t *testing.T) {
		t.Parallel()

		target := &Target{SubjectTmpl: subjectTemplate(t)}
		message, err := target.Render(payload())
		require.Error(t, err)
		assert.Empty(t, message)
	})

	t.Run("requires subject template", func(t *testing.T) {
		t.Parallel()

		target := &Target{Template: bodyTemplate(t, `hello`)}
		message, err := target.Render(payload())
		require.Error(t, err)
		assert.Empty(t, message)
	})

	t.Run("renders subject and body", func(t *testing.T) {
		t.Parallel()

		target := validTarget(t)
		message, err := target.Render(payload())
		require.NoError(t, err)
		assert.Equal(t, "subject ops", message.Subject)
		assert.Equal(t, "hello subject ops", message.Body)
	})
}

// TestSendSMTP tests expected behavior.
func TestSendSMTP(t *testing.T) {
	t.Parallel()

	t.Run("returns dial error", func(t *testing.T) {
		t.Parallel()

		err := sendSMTP(Target{Host: "127.0.0.1", Port: 1}, "subject", "body")
		require.Error(t, err)
	})

	t.Run("sends message", func(t *testing.T) {
		t.Parallel()

		host, port, messages, stop := startSMTPServer(t)
		defer stop()

		err := sendSMTP(Target{Host: host, Port: port, From: "from@example.com", To: []string{"to@example.com"}}, "subject", "body")
		require.NoError(t, err)
		assert.Contains(t, <-messages, "Subject: subject")
	})
}

// TestSMTPTLSConfig tests expected behavior.
func TestSMTPTLSConfig(t *testing.T) {
	t.Parallel()

	cfg := smtpTLSConfig(Target{Host: "smtp.example.com", SkipTLSVerify: true})
	require.NotNil(t, cfg)
	assert.Equal(t, "smtp.example.com", cfg.ServerName)
	assert.True(t, cfg.InsecureSkipVerify)
}

// TestSMTPAuth tests expected behavior.
func TestSMTPAuth(t *testing.T) {
	t.Parallel()

	t.Run("returns nil without credentials", func(t *testing.T) {
		t.Parallel()

		err := smtpAuth(nil, Target{})
		require.NoError(t, err)
	})

	t.Run("returns auth error when server has no auth", func(t *testing.T) {
		t.Parallel()

		host, port, _, stop := startSMTPServer(t)
		defer stop()

		client, err := smtp.Dial(net.JoinHostPort(host, strconv.Itoa(port)))
		require.NoError(t, err)
		defer client.Close() // nolint:errcheck

		err = smtpAuth(client, Target{Host: host, User: "user", Pass: "pass"})
		require.Error(t, err)
	})
}

// TestSMTPSend tests expected behavior.
func TestSMTPSend(t *testing.T) {
	t.Parallel()

	host, port, messages, stop := startSMTPServer(t)
	defer stop()

	client, err := smtp.Dial(net.JoinHostPort(host, strconv.Itoa(port)))
	require.NoError(t, err)
	defer client.Close() // nolint:errcheck

	err = smtpSend(client, "from@example.com", []string{"to@example.com"}, nil, nil, []byte("Subject: hello\r\n\r\nbody\r\n"))
	require.NoError(t, err)
	assert.Contains(t, <-messages, "body")
}

// TestEnvelopeRecipients tests expected behavior.
func TestEnvelopeRecipients(t *testing.T) {
	t.Parallel()

	recipients := envelopeRecipients(
		[]string{"to@example.com"},
		[]string{"cc@example.com"},
		[]string{"bcc@example.com"},
	)

	assert.Equal(t, []string{"to@example.com", "cc@example.com", "bcc@example.com"}, recipients)
}

// TestValidateSMTPConfig tests expected behavior.
func TestValidateSMTPConfig(t *testing.T) {
	t.Parallel()

	t.Run("accepts complete config", func(t *testing.T) {
		t.Parallel()

		err := validateSMTPConfig(Target{Host: "smtp.example.com", Port: 587, From: "from@example.com", To: []string{"to@example.com"}})
		require.NoError(t, err)
	})

	t.Run("requires host", func(t *testing.T) {
		t.Parallel()

		err := validateSMTPConfig(Target{Port: 587, From: "from@example.com", To: []string{"to@example.com"}})
		require.Error(t, err)
	})

	t.Run("requires sender", func(t *testing.T) {
		t.Parallel()

		err := validateSMTPConfig(Target{Host: "smtp.example.com", Port: 587, To: []string{"to@example.com"}})
		require.Error(t, err)
	})

	t.Run("requires recipient", func(t *testing.T) {
		t.Parallel()

		err := validateSMTPConfig(Target{Host: "smtp.example.com", Port: 587, From: "from@example.com"})
		require.Error(t, err)
	})
}

// TestBuildEmail tests expected behavior.
func TestBuildEmail(t *testing.T) {
	t.Parallel()

	message := buildEmail(Target{
		From:    "from@example.com",
		To:      []string{"a@example.com", "b@example.com"},
		CC:      []string{"cc@example.com"},
		BCC:     []string{"bcc@example.com"},
		Headers: map[string]string{"X-B": "2", "X-A": "1"},
	}, "subject", "body")

	text := string(message)
	assert.Contains(t, text, "From: from@example.com\r\n")
	assert.Contains(t, text, "To: a@example.com, b@example.com\r\n")
	assert.Contains(t, text, "Cc: cc@example.com\r\n")
	assert.NotContains(t, text, "Bcc:")
	assert.NotContains(t, text, "bcc@example.com")
	assert.Contains(t, text, "Subject: subject\r\n")
	assert.Contains(t, text, "\r\n\r\nbody\r\n")
	assert.Less(t, strings.Index(text, "X-A: 1"), strings.Index(text, "X-B: 2"))
}

// TestAppendHeaders tests expected behavior.
func TestAppendHeaders(t *testing.T) {
	t.Parallel()

	t.Run("returns existing lines without headers", func(t *testing.T) {
		t.Parallel()

		lines := appendHeaders([]string{"A: 1"}, nil)
		assert.Equal(t, []string{"A: 1"}, lines)
	})

	t.Run("appends sorted headers", func(t *testing.T) {
		t.Parallel()

		lines := appendHeaders([]string{"A: 1"}, map[string]string{"C": "3", "B": "2"})
		assert.Equal(t, []string{"A: 1", "B: 2", "C: 3"}, lines)
	})
}

// payload supports tests.
func payload() notify.Payload {
	return notify.Payload{Notification: testNotification{}, Receiver: "ops", Vars: map[string]any{"team": "platform"}}
}

// validTarget supports tests.
func validTarget(t *testing.T) *Target {
	t.Helper()
	return New(
		WithHost("127.0.0.1"),
		WithPort(1),
		WithFrom("from@example.com"),
		WithTo("to@example.com"),
		WithTemplate(bodyTemplate(t, `hello {{ .Subject }}`)),
		WithSubjectTemplate(subjectTemplate(t)),
	)
}

// subjectTemplate supports tests.
func subjectTemplate(t *testing.T) *templates.StringTemplate {
	t.Helper()
	tmpl, err := templates.ParseStringTemplate("subject", `subject {{ .Receiver }}`)
	require.NoError(t, err)
	return tmpl
}

// bodyTemplate supports tests.
func bodyTemplate(t *testing.T, value string) *templates.Template {
	t.Helper()
	tmpl, err := templates.ParseTemplate("body", value)
	require.NoError(t, err)
	return tmpl
}

// startSMTPServer supports tests.
func startSMTPServer(t *testing.T) (host string, port int, messages <-chan string, stop func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	messageCh := make(chan string, 4)
	var wg sync.WaitGroup
	closed := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-closed:
					return
				default:
					return
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				handleSMTPConn(conn, messageCh)
			}()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, messageCh, func() {
		close(closed)
		_ = listener.Close()
		wg.Wait()
	}
}

// handleSMTPConn supports tests.
func handleSMTPConn(conn net.Conn, messages chan<- string) {
	defer conn.Close() // nolint:errcheck
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTP(writer, "220 localhost ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO"):
			writeSMTP(writer, "250-localhost")
			writeSMTP(writer, "250 OK")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			writeSMTP(writer, "250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			writeSMTP(writer, "250 OK")
		case strings.HasPrefix(cmd, "DATA"):
			writeSMTP(writer, "354 End data with <CR><LF>.<CR><LF>")
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				body.WriteString(dataLine)
			}
			messages <- body.String()
			writeSMTP(writer, "250 OK")
		case strings.HasPrefix(cmd, "QUIT"):
			writeSMTP(writer, "221 Bye")
			return
		default:
			writeSMTP(writer, fmt.Sprintf("250 OK %s", cmd))
		}
	}
}

// writeSMTP supports tests.
func writeSMTP(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}

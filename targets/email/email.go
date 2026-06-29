package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/containeroo/notifykit/internal/header"
	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"
)

// Option configures an email target.
type Option func(*Target)

// Target delivers notifications via SMTP.
type Target struct {
	// Name is an optional human-readable target name used in logs.
	Name string

	// Host is the SMTP server hostname or IP address.
	Host string

	// Port is the SMTP server port. New defaults it to 587 when unset.
	Port int

	// User is the optional SMTP username used for authentication.
	User string

	// Pass is the optional SMTP password used for authentication.
	Pass string

	// From is the envelope sender and message From header.
	From string

	// To contains primary message recipients.
	To []string

	// CC contains carbon-copy message recipients.
	CC []string

	// BCC contains blind-carbon-copy envelope recipients.
	//
	// BCC recipients are passed to the SMTP server as envelope recipients, but
	// they are not written to the message headers.
	BCC []string

	// Headers contains additional message headers.
	//
	// Headers are appended in deterministic key order after the standard message
	// headers. Standard message headers such as From, To, Subject, MIME-Version,
	// and Content-Type cannot be overridden here.
	Headers map[string]string

	// SkipTLSVerify disables SMTP STARTTLS certificate verification.
	//
	// This should only be used for local development or trusted private SMTP
	// servers with self-signed certificates.
	SkipTLSVerify bool

	// DialTimeout limits how long SMTP connection establishment may take.
	//
	// New defaults DialTimeout to 10 seconds when unset.
	DialTimeout time.Duration

	// Template renders the HTML email body.
	Template *templates.Template

	// SubjectTmpl renders the email subject.
	SubjectTmpl *templates.StringTemplate
}

// New constructs an email target from options.
//
// It applies SMTP defaults for optional fields:
//
//   - Port defaults to 587.
//   - DialTimeout defaults to 10 seconds.
//
// Template and SubjectTmpl are not validated by New. They are rendered by
// Render, Validate, Send, or SendResult, which return errors for incomplete
// configuration.
//
// The returned target is safe to pass to notify.Receiver.Targets.
func New(opts ...Option) *Target {
	target := Target{}
	for _, opt := range opts {
		if opt != nil {
			opt(&target)
		}
	}
	applyDefaults(&target)
	return &target
}

// NewFromTarget constructs an email target from an existing Target value.
//
// Additional options are applied after the initial target value, then defaults
// are filled in the same way as New.
func NewFromTarget(target Target, opts ...Option) *Target {
	for _, opt := range opts {
		if opt != nil {
			opt(&target)
		}
	}
	applyDefaults(&target)
	return &target
}

func applyDefaults(target *Target) {
	if target.Port == 0 {
		target.Port = 587
	}
	if target.DialTimeout == 0 {
		target.DialTimeout = 10 * time.Second
	}
}

// WithName configures the human-readable target name used in logs.
func WithName(name string) Option {
	return func(target *Target) { target.Name = name }
}

// WithHost configures the SMTP server hostname or IP address.
func WithHost(host string) Option {
	return func(target *Target) { target.Host = host }
}

// WithPort configures the SMTP server port.
func WithPort(port int) Option {
	return func(target *Target) { target.Port = port }
}

// WithCredentials configures SMTP username and password authentication.
func WithCredentials(user, pass string) Option {
	return func(target *Target) {
		target.User = user
		target.Pass = pass
	}
}

// WithFrom configures the envelope sender and message From header.
func WithFrom(from string) Option {
	return func(target *Target) { target.From = from }
}

// WithTo configures primary message recipients.
func WithTo(recipients ...string) Option {
	return func(target *Target) { target.To = append([]string{}, recipients...) }
}

// WithCC configures carbon-copy message recipients.
func WithCC(recipients ...string) Option {
	return func(target *Target) { target.CC = append([]string{}, recipients...) }
}

// WithBCC configures blind-carbon-copy envelope recipients.
func WithBCC(recipients ...string) Option {
	return func(target *Target) { target.BCC = append([]string{}, recipients...) }
}

// WithHeader configures one additional message header.
func WithHeader(name, value string) Option {
	return WithHeaders(map[string]string{name: value})
}

// WithHeaders configures additional message headers.
func WithHeaders(headers map[string]string) Option {
	return func(target *Target) {
		if len(headers) == 0 {
			return
		}
		if target.Headers == nil {
			target.Headers = map[string]string{}
		}
		maps.Copy(target.Headers, headers)
	}
}

// WithSkipTLSVerify disables SMTP STARTTLS certificate verification.
//
// This should only be used for local development or trusted private SMTP
// servers with self-signed certificates.
func WithSkipTLSVerify() Option {
	return func(target *Target) { target.SkipTLSVerify = true }
}

// WithDialTimeout configures the SMTP connection timeout.
func WithDialTimeout(timeout time.Duration) Option {
	return func(target *Target) { target.DialTimeout = timeout }
}

// WithTemplate configures the HTML email body template.
func WithTemplate(tmpl *templates.Template) Option {
	return func(target *Target) { target.Template = tmpl }
}

// WithSubjectTemplate configures the email subject template.
func WithSubjectTemplate(tmpl *templates.StringTemplate) Option {
	return func(target *Target) { target.SubjectTmpl = tmpl }
}

// Type returns the target type name.
func (t *Target) Type() string { return "email" }

// Send renders and sends an email notification.
func (t *Target) Send(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	return t.SendResult(ctx, payload)
}

// SendResult renders and sends an email notification with delivery details.
func (t *Target) SendResult(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	if t == nil {
		return notify.DeliveryResult{}, errors.New("email target is nil")
	}
	start := time.Now()
	err := contextError(ctx)
	var message Message
	if err == nil {
		message, err = t.Render(payload)
	}
	if err == nil {
		err = sendSMTP(ctx, *t, message.Subject, message.Body)
	}

	status := "sent"
	if err != nil {
		status = "failed"
	}
	return notify.DeliveryResult{
		Status:   status,
		Response: time.Since(start).Round(time.Millisecond).String(),
	}, err
}

// Validate renders the target and validates SMTP settings without sending it.
func (t *Target) Validate(payload notify.Payload) error {
	if t == nil {
		return errors.New("email target is nil")
	}
	if _, err := t.Render(payload); err != nil {
		return err
	}
	return validateSMTPConfig(*t)
}

// Render renders the configured subject and body templates.
func (t *Target) Render(payload notify.Payload) (Message, error) {
	if t == nil {
		return Message{}, errors.New("email target is nil")
	}
	if t.Template == nil {
		return Message{}, errors.New("email template is nil")
	}
	if t.SubjectTmpl == nil {
		return Message{}, errors.New("email subject template is nil")
	}

	subject, err := t.SubjectTmpl.Render(payload.Data(""))
	if err != nil {
		return Message{}, fmt.Errorf("render email subject: %w", err)
	}
	body, err := t.Template.Render(payload.Data(subject))
	if err != nil {
		return Message{}, fmt.Errorf("render email template: %w", err)
	}
	return Message{Subject: subject, Body: string(body)}, nil
}

// Message contains a rendered email subject and body.
type Message struct {
	Subject string
	Body    string
}

// sendSMTP sends a rendered email through the configured SMTP server.
func sendSMTP(ctx context.Context, target Target, subject, body string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateSMTPConfig(target); err != nil {
		return err
	}

	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	msg := buildEmail(target, subject, body)

	dialer := net.Dialer{Timeout: smtpDialTimeout(target)}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close() // nolint:errcheck

	stopContextClose := closeConnOnContextDone(ctx, conn)
	defer stopContextClose()

	client, err := smtp.NewClient(conn, target.Host)
	if err != nil {
		return err
	}
	defer client.Close() // nolint:errcheck

	if err := contextError(ctx); err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(smtpTLSConfig(target)); err != nil {
			return err
		}
	}

	if err := contextError(ctx); err != nil {
		return err
	}
	if err := smtpAuth(client, target); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return smtpSend(client, target.From, target.To, target.CC, target.BCC, msg)
}

// contextError returns the current context error or a nil-context error.
func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is nil")
	}
	return ctx.Err()
}

// smtpDialTimeout returns the configured connection timeout.
func smtpDialTimeout(target Target) time.Duration {
	if target.DialTimeout <= 0 {
		return 10 * time.Second
	}
	return target.DialTimeout
}

// closeConnOnContextDone closes conn if ctx is canceled during SMTP operations.
func closeConnOnContextDone(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// validateSMTPConfig reports missing or invalid SMTP delivery settings.
func validateSMTPConfig(target Target) error {
	if strings.TrimSpace(target.Host) == "" {
		return errors.New("email host is required")
	}
	if target.Port <= 0 {
		return errors.New("email port must be greater than zero")
	}
	if strings.TrimSpace(target.From) == "" {
		return errors.New("email from address is required")
	}
	if len(envelopeRecipients(target.To, target.CC, target.BCC)) == 0 {
		return errors.New("email recipient is required")
	}
	if err := validateHeaders(target.Headers); err != nil {
		return err
	}
	return nil
}

// validateHeaders validates custom message headers.
func validateHeaders(headers map[string]string) error {
	for name, value := range headers {
		if err := validateHeaderName(name); err != nil {
			return err
		}
		if isReservedHeader(name) {
			return fmt.Errorf("email header %q is reserved", name)
		}
		if err := validateHeaderValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

// validateHeaderName reports whether name is safe for use as an email header field name.
func validateHeaderName(name string) error {
	if name == "" {
		return errors.New("email header name must not be empty")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("email header %q must not have leading or trailing whitespace", name)
	}
	if !header.ValidFieldName(name) {
		return fmt.Errorf("email header %q contains invalid character", name)
	}
	return nil
}

// isReservedHeader reports whether name would override a standard header.
func isReservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "from", "to", "cc", "bcc", "subject", "mime-version", "content-type":
		return true
	default:
		return false
	}
}

// validateHeaderValue reports whether value is safe for use as an email header value.
func validateHeaderValue(name, value string) error {
	if header.ContainsNewline(value) {
		return fmt.Errorf("email header %q value must not contain newline characters", name)
	}
	return nil
}

// smtpTLSConfig returns the TLS configuration for STARTTLS.
func smtpTLSConfig(target Target) *tls.Config {
	return &tls.Config{
		ServerName:         target.Host,
		InsecureSkipVerify: target.SkipTLSVerify, // nolint:gosec // Explicitly controlled by caller configuration.
	}
}

// smtpAuth authenticates the SMTP client when credentials are configured.
func smtpAuth(client *smtp.Client, target Target) error {
	if target.User == "" && target.Pass == "" {
		return nil
	}
	if client == nil {
		return errors.New("smtp client is nil")
	}
	return client.Auth(smtp.PlainAuth("", target.User, target.Pass, target.Host))
}

// smtpSend writes the message through an initialized SMTP client.
func smtpSend(client *smtp.Client, from string, to, cc, bcc []string, msg []byte) error {
	if client == nil {
		return errors.New("smtp client is nil")
	}
	if err := client.Mail(from); err != nil {
		return err
	}

	for _, recipient := range envelopeRecipients(to, cc, bcc) {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

// envelopeRecipients returns all SMTP envelope recipients.
func envelopeRecipients(to, cc, bcc []string) []string {
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)
	return recipients
}

// buildEmail returns a raw RFC 5322 style email message.
func buildEmail(target Target, subject, body string) []byte {
	headers := []string{
		"From: " + target.From,
		"To: " + strings.Join(target.To, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
	}

	if len(target.CC) > 0 {
		headers = append(headers, "Cc: "+strings.Join(target.CC, ", "))
	}

	headers = appendHeaders(headers, target.Headers)

	var buf strings.Builder
	for _, line := range headers {
		if strings.TrimSpace(line) == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\r\n")
	}
	buf.WriteString("\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n")
	return []byte(buf.String())
}

// appendHeaders appends custom headers in deterministic order.
func appendHeaders(lines []string, headers map[string]string) []string {
	if len(headers) == 0 {
		return lines
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, name+": "+headers[name])
	}
	return lines
}

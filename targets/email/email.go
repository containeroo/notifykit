package email

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/containeroo/notifykit/internal/header"
	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"
)

// TLSMode selects how SMTP traffic is encrypted.
type TLSMode string

const (
	// TLSRequired requires STARTTLS and fails before sending if unavailable.
	TLSRequired TLSMode = "starttls"
	// TLSImplicit establishes TLS before the SMTP greeting (usually port 465).
	TLSImplicit TLSMode = "implicit"
	// TLSPlaintext explicitly disables TLS, for local or otherwise secured relays.
	TLSPlaintext TLSMode = "plaintext"
)

// BodyFormat selects the MIME representation of the rendered email body.
type BodyFormat string

const (
	// BodyHTML sends the rendered body as text/html.
	BodyHTML BodyFormat = "html"
	// BodyText sends the rendered body as text/plain.
	BodyText BodyFormat = "text"
)

// WithTLSMode configures SMTP encryption. The default is TLSRequired.
func WithTLSMode(mode TLSMode) Option { return func(t *Target) { t.TLSMode = mode } }

// WithBodyFormat configures the rendered body's MIME format. The default is BodyHTML.
func WithBodyFormat(format BodyFormat) Option { return func(t *Target) { t.BodyFormat = format } }

// Option configures an email target.
type Option func(*Target)

// Target delivers notifications via SMTP.
type Target struct {
	// TLSMode defaults to TLSRequired.
	TLSMode TLSMode
	// BodyFormat defaults to BodyHTML for backward compatibility.
	BodyFormat BodyFormat
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
	// Timeout bounds the complete SMTP attempt, including TLS and message transfer.
	// Nonpositive values default to 30 seconds. An earlier caller deadline wins.
	Timeout time.Duration

	// ProxyFromEnvironment enables HTTP_PROXY, HTTPS_PROXY, and NO_PROXY for SMTP tunnels.
	ProxyFromEnvironment bool

	// Template renders the email body.
	Template templates.Renderer

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
	target.Headers = maps.Clone(target.Headers)
	target.To = slices.Clone(target.To)
	target.CC = slices.Clone(target.CC)
	target.BCC = slices.Clone(target.BCC)
	for _, opt := range opts {
		if opt != nil {
			opt(&target)
		}
	}
	applyDefaults(&target)
	return &target
}

func applyDefaults(target *Target) {
	target.TLSMode = cmp.Or(target.TLSMode, TLSRequired)
	target.BodyFormat = cmp.Or(target.BodyFormat, BodyHTML)
	if target.Port == 0 {
		if target.TLSMode == TLSImplicit {
			target.Port = 465
		} else {
			target.Port = 587
		}
	}
	if target.Timeout <= 0 {
		target.Timeout = 30 * time.Second
	}
	if target.DialTimeout <= 0 {
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
	return func(target *Target) { target.To = slices.Clone(recipients) }
}

// WithCC configures carbon-copy message recipients.
func WithCC(recipients ...string) Option {
	return func(target *Target) { target.CC = slices.Clone(recipients) }
}

// WithBCC configures blind-carbon-copy envelope recipients.
func WithBCC(recipients ...string) Option {
	return func(target *Target) { target.BCC = slices.Clone(recipients) }
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

// WithTimeout bounds the complete SMTP attempt (default 30 seconds).
func WithTimeout(timeout time.Duration) Option { return func(t *Target) { t.Timeout = timeout } }

// WithProxyFromEnvironment makes SMTP delivery honor HTTP_PROXY, HTTPS_PROXY, and NO_PROXY.
func WithProxyFromEnvironment() Option {
	return func(target *Target) { target.ProxyFromEnvironment = true }
}

// WithTemplate configures the email body template.
func WithTemplate(tmpl templates.Renderer) Option {
	return func(target *Target) { target.Template = tmpl }
}

// WithSubjectTemplate configures the email subject template.
func WithSubjectTemplate(tmpl *templates.StringTemplate) Option {
	return func(target *Target) { target.SubjectTmpl = tmpl }
}

// Type returns the target type name.
func (t *Target) Type() string { return "email" }

// ProxyAddress returns the selected proxy authority without credentials.
func (t *Target) ProxyAddress() (string, error) {
	if t == nil || !t.ProxyFromEnvironment {
		return "", nil
	}
	target := *t
	applyDefaults(&target)
	address := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	proxy, err := resolveProxy(address, target.TLSMode)
	if err != nil || proxy == nil {
		return "", err
	}
	return proxy.Host, nil
}

// Send renders and sends an email notification.
func (t *Target) Send(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	return t.SendResult(ctx, payload)
}

// SendResult renders and sends an email notification with delivery details.
func (t *Target) SendResult(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	if t == nil {
		return notify.DeliveryResult{}, notify.Permanent(errors.New("email target is nil"))
	}
	if ctx == nil {
		return notify.DeliveryResult{}, notify.Permanent(errors.New("context is nil"))
	}
	if err := ctx.Err(); err != nil {
		return notify.DeliveryResult{}, err
	}

	target := *t
	applyDefaults(&target)

	start := time.Now()
	message, err := target.Render(payload)
	if err == nil {
		err = validateSMTPConfig(target)
	}
	if err != nil {
		err = notify.Permanent(err)
	} else {
		err = classifySMTPError(sendSMTP(ctx, target, message.Subject, message.Body))
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

	target := *t
	applyDefaults(&target)

	if _, err := target.Render(payload); err != nil {
		return err
	}
	return validateSMTPConfig(target)
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
	if err := validateHeaderValue("Subject", subject); err != nil {
		return Message{}, err
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

// sendSMTP sends a validated rendered email through the configured SMTP server.
func sendSMTP(ctx context.Context, target Target, subject, body string) (resultErr error) {
	applyDefaults(&target)
	ctx, cancel := context.WithTimeout(ctx, target.Timeout)
	defer cancel()
	defer func() {
		// A successful DATA acknowledgement remains successful even if the
		// context expires while the connection is being closed.
		if resultErr != nil && ctx.Err() != nil {
			resultErr = ctx.Err()
		}
	}()

	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	msg := buildEmail(target, subject, body)

	conn, err := dialSMTPConnection(ctx, target, addr)
	if err != nil {
		return operationError("connection", target.Timeout, err)
	}
	defer conn.Close() // nolint:errcheck
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return operationError("connection deadline", target.Timeout, err)
		}
	}
	if target.TLSMode == TLSImplicit {
		secured := tls.Client(conn, smtpTLSConfig(target))
		if err := secured.HandshakeContext(ctx); err != nil {
			return operationError("TLS handshake", target.Timeout, err)
		}
		conn = secured
	}

	stopContextClose := closeConnOnContextDone(ctx, conn)
	defer stopContextClose()

	client, err := smtp.NewClient(conn, target.Host)
	if err != nil {
		return operationError("server greeting", target.Timeout, err)
	}
	defer client.Close() // nolint:errcheck

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Hello("localhost"); err != nil {
		return operationError("EHLO/HELO", target.Timeout, err)
	}
	if target.TLSMode != TLSPlaintext && target.TLSMode != TLSImplicit {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return notify.Permanent(errors.New("SMTP server does not support required STARTTLS"))
		}
		if err := client.StartTLS(smtpTLSConfig(target)); err != nil {
			return operationError("STARTTLS handshake", target.Timeout, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := smtpAuth(client, target); err != nil {
		return operationError("authentication", target.Timeout, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return smtpSend(client, target, msg)
}

// dialSMTPConnection opens either a direct TCP connection or an HTTP CONNECT tunnel.
func dialSMTPConnection(ctx context.Context, target Target, address string) (net.Conn, error) {
	if !target.ProxyFromEnvironment {
		return (&net.Dialer{Timeout: target.DialTimeout}).DialContext(ctx, "tcp", address)
	}

	proxy, err := resolveProxy(address, target.TLSMode)
	if err != nil {
		return nil, err
	}
	if proxy == nil {
		return (&net.Dialer{Timeout: target.DialTimeout}).DialContext(ctx, "tcp", address)
	}
	return dialProxy(ctx, proxy, address, target.DialTimeout)
}

// OperationError identifies the SMTP stage that failed while preserving the underlying cause.
type OperationError struct {
	// Operation is the human-readable SMTP stage.
	Operation string
	// Timeout is the configured complete-attempt timeout.
	Timeout time.Duration
	// Err is the underlying transport or SMTP error.
	Err error
}

// Error returns a concise operator-facing SMTP failure.
func (e *OperationError) Error() string {
	if e == nil {
		return "SMTP operation failed"
	}
	if isTimeout(e.Err) {
		return fmt.Sprintf("SMTP %s timed out after %s", e.Operation, e.Timeout)
	}
	return fmt.Sprintf("SMTP %s failed: %v", e.Operation, e.Err)
}

// Unwrap exposes the original cause for retry classification.
func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// operationError adds the failing SMTP stage while preserving the original cause.
func operationError(operation string, timeout time.Duration, err error) error {
	if err == nil {
		return nil
	}
	return &OperationError{Operation: operation, Timeout: timeout, Err: err}
}

// isTimeout reports whether an error chain represents an operation timeout.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var network net.Error
	return errors.As(err, &network) && network.Timeout()
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
	switch target.TLSMode {
	case "", TLSRequired, TLSImplicit, TLSPlaintext:
	default:
		return errors.New("invalid SMTP TLS mode")
	}
	switch target.BodyFormat {
	case "", BodyHTML, BodyText:
	default:
		return errors.New("invalid email body format")
	}
	if target.TLSMode == TLSPlaintext && (target.User != "" || target.Pass != "") {
		return errors.New("SMTP credentials require TLS")
	}
	if strings.TrimSpace(target.Host) == "" {
		return errors.New("email host is required")
	}
	if target.Port <= 0 {
		return errors.New("email port must be greater than zero")
	}
	if target.Port > 65535 {
		return errors.New("email port must not exceed 65535")
	}
	if strings.TrimSpace(target.From) == "" {
		return errors.New("email from address is required")
	}
	if len(envelopeRecipients(target.To, target.CC, target.BCC)) == 0 {
		return errors.New("email recipient is required")
	}
	if _, err := parseMailbox(target.From); err != nil {
		return fmt.Errorf("email from: %w", err)
	}
	for _, recipient := range envelopeRecipients(target.To, target.CC, target.BCC) {
		if _, err := parseMailbox(recipient); err != nil {
			return fmt.Errorf("email recipient: %w", err)
		}
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
	case "from", "to", "cc", "bcc", "subject", "mime-version", "content-type", "content-transfer-encoding":
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

// smtpTLSConfig returns the TLS configuration for SMTP encryption.
func smtpTLSConfig(target Target) *tls.Config {
	return &tls.Config{
		ServerName:         target.Host,
		InsecureSkipVerify: target.SkipTLSVerify, // nolint:gosec // Explicitly controlled by caller configuration.
		MinVersion:         tls.VersionTLS12,
	}
}

// smtpAuth authenticates the SMTP client when credentials are configured.
func smtpAuth(client *smtp.Client, target Target) error {
	if target.User == "" && target.Pass == "" {
		return nil
	}
	return client.Auth(smtp.PlainAuth("", target.User, target.Pass, target.Host))
}

// smtpSend writes the message through an initialized SMTP client.
func smtpSend(client *smtp.Client, target Target, msg []byte) error {
	from, err := parseMailbox(target.From)
	if err != nil {
		return operationError("MAIL FROM", target.Timeout, err)
	}
	if err := client.Mail(from.Address); err != nil {
		return operationError("MAIL FROM", target.Timeout, err)
	}

	for _, recipient := range envelopeRecipients(target.To, target.CC, target.BCC) {
		mailbox, err := parseMailbox(recipient)
		if err != nil {
			return operationError("RCPT TO", target.Timeout, err)
		}
		if err := client.Rcpt(mailbox.Address); err != nil {
			return operationError("RCPT TO", target.Timeout, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return operationError("message data", target.Timeout, err)
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return operationError("message transfer", target.Timeout, err)
	}
	if err := writer.Close(); err != nil {
		return operationError("message acceptance", target.Timeout, err)
	}
	return nil
}

// envelopeRecipients returns all SMTP envelope recipients.
func envelopeRecipients(to, cc, bcc []string) []string {
	return slices.Concat(to, cc, bcc)
}

// buildEmail returns a raw RFC 5322 style email message.
func buildEmail(target Target, subject, body string) []byte {
	headers := []string{
		"From: " + formatMailbox(target.From),
		"To: " + formatMailboxes(target.To),
		"Subject: " + encodeSubject(subject),
		"MIME-Version: 1.0",
		"Content-Type: " + bodyContentType(target.BodyFormat) + "; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
	}

	if len(target.CC) > 0 {
		headers = append(headers, "Cc: "+formatMailboxes(target.CC))
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
	normalizedBody := normalizeBody(body)
	buf.WriteString(normalizedBody)
	if !strings.HasSuffix(normalizedBody, "\r\n") {
		buf.WriteString("\r\n")
	}
	return []byte(buf.String())
}

// bodyContentType returns the MIME content type for a configured body format.
func bodyContentType(format BodyFormat) string {
	if format == BodyText {
		return "text/plain"
	}
	return "text/html"
}

// formatMailbox canonicalizes a validated mailbox for a message header.
func formatMailbox(value string) string {
	mailbox, err := parseMailbox(value)
	if err != nil {
		return value
	}
	return mailbox.String()
}

// formatMailboxes canonicalizes validated mailboxes for a message header.
func formatMailboxes(values []string) string {
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, formatMailbox(value))
	}
	return strings.Join(formatted, ", ")
}

// encodeSubject MIME-encodes a non-ASCII subject.
func encodeSubject(value string) string {
	if isASCII(value) {
		return value
	}
	return mime.QEncoding.Encode("UTF-8", value)
}

// isASCII reports whether value can be emitted directly in an RFC 5322 header.
func isASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return false
		}
	}
	return true
}

// normalizeBody converts line endings to SMTP-friendly CRLF.
func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}

// appendHeaders appends custom headers in deterministic order.
func appendHeaders(lines []string, headers map[string]string) []string {
	if len(headers) == 0 {
		return lines
	}
	names := slices.Sorted(maps.Keys(headers))
	for _, name := range names {
		lines = append(lines, name+": "+headers[name])
	}
	return lines
}

// parseMailbox validates one mailbox and returns its parsed envelope address.
func parseMailbox(value string) (*mail.Address, error) {
	if header.ContainsNewline(value) {
		return nil, errors.New("address must not contain newline characters")
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || parsed.Address == "" {
		return nil, errors.New("address must be a single mailbox")
	}
	return parsed, nil
}

// classifySMTPError keeps SMTP reply codes separate from HTTP status codes.
// Temporary SMTP replies are transport failures; permanent replies cannot retry.
func classifySMTPError(err error) error {
	if err == nil || notify.IsPermanent(err) {
		return err
	}
	var proxy *ProxyError
	if errors.As(err, &proxy) {
		if proxy.StatusCode == 408 || proxy.StatusCode == 429 || proxy.StatusCode >= 500 {
			return notify.Transport(err)
		}
		return notify.Permanent(err)
	}
	var reply *textproto.Error
	if errors.As(err, &reply) {
		if reply.Code >= 400 && reply.Code < 500 {
			return notify.Transport(err)
		}
		return notify.Permanent(err)
	}
	var verification *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalidCertificate x509.CertificateInvalidError
	if errors.As(err, &verification) || errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalidCertificate) {
		return notify.Permanent(err)
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return notify.Transport(err)
	}
	// Local protocol/authentication failures are not transient by default.
	return notify.Permanent(err)
}

package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/containeroo/notifykit/internal/header"
	"github.com/containeroo/notifykit/notify"
	"github.com/containeroo/notifykit/templates"
)

// LogResponse controls how much webhook response information is logged.
type LogResponse string

const (
	// LogResponseSummary logs only status, status code, duration, and truncation state.
	LogResponseSummary LogResponse = "summary"

	// LogResponseBody logs status fields and response body, but not response headers.
	LogResponseBody LogResponse = "body"

	// LogResponseFull logs status fields, response body, and response headers.
	LogResponseFull LogResponse = "full"

	// LogResponseNone suppresses successful webhook response logs.
	//
	// Error responses are still logged with summary fields, but without response
	// bodies or headers.
	LogResponseNone LogResponse = "none"
)

// Option configures a webhook target.
type Option func(*Target)

// ClientOption configures the HTTP client created by NewClient.
type ClientOption func(*clientOptions)

type clientOptions struct {
	proxyFromEnvironment bool
	skipTLSVerify        bool
}

// Target delivers notifications to a webhook endpoint.
type Target struct {
	// Name is an optional human-readable target name used in logs.
	//
	// When Name is empty, logs and delivery errors use a generic target label.
	//
	// The raw URL is intentionally not used as a fallback because webhook URLs
	// commonly contain secrets.
	Name string

	// URL is the webhook endpoint URL.
	URL string

	// Method is the HTTP method used for webhook requests.
	//
	// New defaults Method to POST when unset.
	Method string

	// Headers contains additional HTTP request headers.
	//
	// Headers are set after the default Content-Type header, so callers may
	// override Content-Type when needed.
	Headers map[string]string

	// Template renders the HTTP request body.
	Template *templates.Template

	// TitleTmpl renders the notification title.
	//
	// The rendered title is passed back into the body template data so body
	// templates can use it as .Title.
	TitleTmpl *templates.StringTemplate

	// Client sends HTTP webhook requests.
	//
	// New defaults Client to NewClient with a 10 second timeout when unset.
	// Provide a custom client for custom transports, proxies, tracing, mTLS,
	// test servers, or different timeout behavior.
	Client *http.Client

	// Logger receives webhook-specific request, response, and error logs.
	//
	// New defaults Logger to a discard logger when unset.
	Logger *slog.Logger

	// ValidateJSON requires the rendered request body to be valid JSON.
	//
	// This is useful for JSON webhook integrations such as Slack, Discord, or
	// custom HTTP APIs. When enabled, Render returns an error before sending if
	// the template output is not valid JSON.
	ValidateJSON bool

	// LogResponse controls how much response information is logged.
	//
	// New defaults LogResponse to LogResponseSummary when unset. Error responses
	// are logged with summary fields by default; response bodies and headers are
	// only logged when LogResponseBody or LogResponseFull is configured.
	LogResponse LogResponse

	// ResponseBodyLimit limits how many response-body bytes are read and logged.
	//
	// New defaults ResponseBodyLimit to 4096 when unset. Values less than or
	// equal to zero are treated as the default by response-body reading.
	ResponseBodyLimit int
}

// New constructs a webhook target from options.
//
// It applies defaults for optional fields:
//
//   - Method defaults to POST.
//   - Client defaults to NewClient with a 10 second timeout and no proxy.
//   - Logger defaults to a discard logger.
//   - LogResponse defaults to LogResponseSummary.
//   - ResponseBodyLimit defaults to 4096 bytes.
//
// Template and TitleTmpl are not validated by New. They are rendered by
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

// NewFromTarget constructs a webhook target from an existing Target value.
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
	if target.Method == "" {
		target.Method = http.MethodPost
	}
	if target.Client == nil {
		target.Client = NewClient(10 * time.Second)
	}
	if target.Logger == nil {
		target.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if target.LogResponse == "" {
		target.LogResponse = LogResponseSummary
	}
	if target.ResponseBodyLimit == 0 {
		target.ResponseBodyLimit = 4096
	}
}

// WithName configures the human-readable target name used in logs.
func WithName(name string) Option {
	return func(target *Target) { target.Name = name }
}

// WithURL configures the webhook endpoint URL.
func WithURL(url string) Option {
	return func(target *Target) { target.URL = url }
}

// WithMethod configures the HTTP method used for webhook requests.
func WithMethod(method string) Option {
	return func(target *Target) { target.Method = method }
}

// WithHeader configures one additional HTTP request header.
func WithHeader(name, value string) Option {
	return WithHeaders(map[string]string{name: value})
}

// WithHeaders configures additional HTTP request headers.
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

// WithTemplate configures the HTTP request body template.
func WithTemplate(tmpl *templates.Template) Option {
	return func(target *Target) { target.Template = tmpl }
}

// WithTitleTemplate configures the notification title template.
func WithTitleTemplate(tmpl *templates.StringTemplate) Option {
	return func(target *Target) { target.TitleTmpl = tmpl }
}

// WithClient configures the HTTP client used to send webhook requests.
func WithClient(client *http.Client) Option {
	return func(target *Target) { target.Client = client }
}

// WithLogger configures the target-specific logger.
func WithLogger(logger *slog.Logger) Option {
	return func(target *Target) { target.Logger = logger }
}

// WithValidateJSON enables validation of the rendered body as JSON.
func WithValidateJSON() Option {
	return func(target *Target) { target.ValidateJSON = true }
}

// WithLogResponse configures how much webhook response information is logged.
func WithLogResponse(mode LogResponse) Option {
	return func(target *Target) { target.LogResponse = mode }
}

// WithResponseBodyLimit configures how many response-body bytes are read and logged.
func WithResponseBodyLimit(limit int) Option {
	return func(target *Target) { target.ResponseBodyLimit = limit }
}

// Type returns the target type name.
func (t *Target) Type() string { return "webhook" }

// Send renders and posts a webhook notification.
func (t *Target) Send(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	return t.SendResult(ctx, payload)
}

// SendResult renders and posts a webhook notification with response details.
func (t *Target) SendResult(ctx context.Context, payload notify.Payload) (notify.DeliveryResult, error) {
	if t == nil {
		return notify.DeliveryResult{}, errors.New("webhook target is nil")
	}
	body, err := t.Render(payload)
	if err != nil {
		return notify.DeliveryResult{}, err
	}

	status, statusCode, responseBody, err := t.post(ctx, body)
	result := notify.DeliveryResult{
		Status:     status,
		StatusCode: statusCode,
		Response:   truncateBody(responseBody, t.ResponseBodyLimit),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// Validate renders the target and validates request settings without sending it.
func (t *Target) Validate(payload notify.Payload) error {
	if t == nil {
		return errors.New("webhook target is nil")
	}
	if _, err := t.Render(payload); err != nil {
		return err
	}
	return validateHeaders(t.Headers)
}

// Render renders the configured title and body templates.
func (t *Target) Render(payload notify.Payload) ([]byte, error) {
	if t == nil {
		return nil, errors.New("webhook target is nil")
	}
	if t.Template == nil {
		return nil, errors.New("webhook template is nil")
	}
	if t.TitleTmpl == nil {
		return nil, errors.New("webhook title template is nil")
	}

	title, err := t.TitleTmpl.Render(payload.Data(""))
	if err != nil {
		return nil, fmt.Errorf("render webhook title: %w", err)
	}

	body, err := t.Template.Render(payload.Data(title))
	if err != nil {
		return nil, fmt.Errorf("render webhook template: %w", err)
	}
	body = bytes.TrimSpace(body)
	if t.ValidateJSON && !json.Valid(body) {
		return nil, fmt.Errorf("render webhook template: result is not valid JSON")
	}
	return body, nil
}

// post sends the rendered body to the configured webhook endpoint.
func (t *Target) post(ctx context.Context, body []byte) (status string, statusCode int, response string, err error) {
	if err := validateHeaders(t.Headers); err != nil {
		return "", 0, "", err
	}

	client := t.Client
	if client == nil {
		client = NewClient(10 * time.Second)
	}
	method := t.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, t.URL, bytes.NewReader(body))
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	for name, value := range t.Headers {
		req.Header.Set(name, value)
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		t.Logger.Error("webhook request failed", "duration", duration.Round(time.Millisecond).String(), "error", err)
		return "", 0, "", err
	}
	defer resp.Body.Close() // nolint:errcheck

	responseBody, truncated, err := readResponseBody(resp.Body, t.ResponseBodyLimit)
	if err != nil {
		t.Logger.Error("webhook response read failed", "status", resp.Status, "statusCode", resp.StatusCode, "duration", duration.Round(time.Millisecond).String(), "error", err)
		return resp.Status, resp.StatusCode, "", err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := responseError(t.label(), resp.Status)
		t.logFailedResponse(resp, responseBody, truncated, duration, err)
		return resp.Status, resp.StatusCode, responseBody, err
	}

	t.logSuccessfulResponse(resp, responseBody, truncated, duration)
	return resp.Status, resp.StatusCode, responseBody, nil
}

// validateHeaders validates custom HTTP request headers.
func validateHeaders(headers map[string]string) error {
	for name, value := range headers {
		if err := validateHeaderName(name); err != nil {
			return err
		}
		if err := validateHeaderValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

// validateHeaderName reports whether name is safe for use as an HTTP header field name.
func validateHeaderName(name string) error {
	if name == "" {
		return errors.New("webhook header name must not be empty")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("webhook header %q must not have leading or trailing whitespace", name)
	}
	if !header.ValidFieldName(name) {
		return fmt.Errorf("webhook header %q contains invalid character", name)
	}
	return nil
}

// validateHeaderValue reports whether value is safe for use as an HTTP header value.
func validateHeaderValue(name, value string) error {
	if header.ContainsNewline(value) {
		return fmt.Errorf("webhook header %q value must not contain newline characters", name)
	}
	return nil
}

// logSuccessfulResponse writes a successful webhook response according to LogResponse.
func (t *Target) logSuccessfulResponse(resp *http.Response, body string, truncated bool, duration time.Duration) {
	switch t.LogResponse {
	case LogResponseNone:
		return
	case LogResponseBody, LogResponseFull:
		t.Logger.Info("webhook delivered", t.responseLogFields(resp, body, truncated, duration, t.LogResponse)...)
	default:
		t.Logger.Info("webhook delivered", t.responseLogFields(resp, body, truncated, duration, LogResponseSummary)...)
	}
}

// logFailedResponse writes a failed webhook response without body or header details unless explicitly enabled.
func (t *Target) logFailedResponse(resp *http.Response, body string, truncated bool, duration time.Duration, err error) {
	mode := t.LogResponse
	switch mode {
	case LogResponseBody, LogResponseFull:
	case LogResponseNone, LogResponseSummary, "":
		mode = LogResponseSummary
	default:
		mode = LogResponseSummary
	}

	t.Logger.Error("webhook delivery failed", append(t.responseLogFields(resp, body, truncated, duration, mode), "error", err)...)
}

// responseLogFields returns structured fields for webhook response logging.
func (t *Target) responseLogFields(resp *http.Response, body string, truncated bool, duration time.Duration, mode LogResponse) []any {
	fields := []any{
		"target", t.label(),
		"status", resp.Status,
		"statusCode", resp.StatusCode,
		"duration", duration.Round(time.Millisecond).String(),
		"responseTruncated", truncated,
	}
	if mode == LogResponseBody || mode == LogResponseFull {
		fields = append(fields, "responseBody", body)
	}
	if mode == LogResponseFull {
		fields = append(fields, "responseHeaders", resp.Header)
	}
	return fields
}

// label returns the configured target name or a secret-safe fallback.
func (t *Target) label() string {
	if t.Name != "" {
		return t.Name
	}
	return "webhook"
}

// NewClient constructs an HTTP client for webhook delivery.
//
// Timeout values less than or equal to zero default to 10 seconds. The returned
// client does not use proxy environment variables unless WithProxyFromEnvironment
// is provided. Use WithSkipTLSVerify to disable TLS certificate verification.
func NewClient(timeout time.Duration, opts ...ClientOption) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cfg := clientOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.skipTLSVerify, // nolint:gosec // Explicitly controlled by caller configuration.
		},
	}
	if cfg.proxyFromEnvironment {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// WithProxyFromEnvironment makes a NewClient-created client honor proxy environment variables.
func WithProxyFromEnvironment() ClientOption {
	return func(cfg *clientOptions) { cfg.proxyFromEnvironment = true }
}

// WithSkipTLSVerify disables TLS certificate verification for a NewClient-created client.
//
// This should only be used for local development or trusted private endpoints
// with self-signed certificates.
func WithSkipTLSVerify() ClientOption {
	return func(cfg *clientOptions) { cfg.skipTLSVerify = true }
}

// readResponseBody reads a response body up to limit bytes.
func readResponseBody(body io.Reader, limit int) (text string, truncated bool, err error) {
	if limit <= 0 {
		limit = 4096
	}
	reader := io.LimitReader(body, int64(limit)+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", false, err
	}
	truncated = len(data) > limit
	if truncated {
		data = data[:limit]
	}
	return strings.TrimSpace(string(data)), truncated, nil
}

// responseError returns a secret-safe delivery error.
func responseError(target, status string) error {
	return fmt.Errorf("webhook %q delivery failed: %s", target, status)
}

// truncateBody shortens a response body to the configured limit.
func truncateBody(body string, limit int) string {
	if limit <= 0 || len(body) <= limit {
		return body
	}
	return body[:limit] + "..."
}

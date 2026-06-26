package templates

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/containeroo/tmplfuncs"
)

const (
	// BuiltinPrefix marks a template source as an embedded built-in template.
	BuiltinPrefix = "builtin:"
)

// MissingKey controls how templates behave when a map key is missing.
type MissingKey string

const (
	// MissingKeyDefault keeps the default text/template behavior and renders "<no value>".
	MissingKeyDefault MissingKey = "default"
	// MissingKeyZero renders the zero value for the missing key's element type.
	MissingKeyZero MissingKey = "zero"
	// MissingKeyError returns an execution error when a key is missing.
	MissingKeyError MissingKey = "error"
)

// Option customizes template parsing.
type Option func(*options)

// options stores parser configuration.
type options struct {
	missingKey MissingKey
	funcs      template.FuncMap
}

// WithMissingKey configures the text/template missingkey option.
func WithMissingKey(policy MissingKey) Option {
	return func(cfg *options) {
		cfg.missingKey = policy
	}
}

// WithFunc adds one custom template function.
func WithFunc(name string, fn any) Option {
	return func(cfg *options) {
		if cfg.funcs == nil {
			cfg.funcs = template.FuncMap{}
		}
		cfg.funcs[name] = fn
	}
}

// WithFuncs adds custom template functions.
func WithFuncs(funcs template.FuncMap) Option {
	return func(cfg *options) {
		if len(funcs) == 0 {
			return
		}
		if cfg.funcs == nil {
			cfg.funcs = template.FuncMap{}
		}
		maps.Copy(cfg.funcs, funcs)
	}
}

// Template wraps a parsed template that renders bytes.
type Template struct {
	tmpl *template.Template
}

// StringTemplate wraps a parsed template that renders strings.
type StringTemplate struct {
	tmpl *template.Template
}

// Templates reads built-in templates from an injected filesystem.
type Templates struct {
	files fs.FS
}

// New creates a built-in template registry backed by files.
//
// The returned registry is used for builtin: template sources. A nil filesystem
// is allowed, but Read and Exists will return an error and Names will return nil.
func New(files fs.FS) Templates {
	return Templates{files: files}
}

// FuncMap returns the shared default template helper functions.
//
// Notifykit exposes all helpers provided by github.com/containeroo/tmplfuncs by
// default. Applications can override or extend helpers with WithFunc or WithFuncs.
func FuncMap() template.FuncMap {
	return tmplfuncs.FuncMap()
}

// IsBuiltin reports whether source references an embedded built-in template.
func IsBuiltin(source string) bool {
	return strings.HasPrefix(strings.TrimSpace(source), BuiltinPrefix)
}

// Name trims the builtin prefix and surrounding whitespace from source.
func Name(source string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(source), BuiltinPrefix))
}

// Read returns an embedded built-in template by name.
func (t Templates) Read(source string) (filename string, body string, err error) {
	name := Name(source)
	if name == "" {
		return "", "", fmt.Errorf("built-in template name must not be empty")
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return "", "", fmt.Errorf("built-in template name %q must not contain path separators", name)
	}
	if t.files == nil {
		return "", "", fmt.Errorf("built-in templates are not configured")
	}

	filename = name + ".tmpl"
	bodyBytes, err := fs.ReadFile(t.files, filename)
	if err != nil {
		return "", "", fmt.Errorf("read built-in template %q: %w", name, err)
	}

	return filename, string(bodyBytes), nil
}

// Exists reports whether the named built-in template can be read.
func (t Templates) Exists(source string) error {
	_, _, err := t.Read(source)
	return err
}

// Names returns all available built-in template names.
func (t Templates) Names() []string {
	if t.files == nil {
		return nil
	}

	entries, err := fs.ReadDir(t.files, ".")
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".tmpl" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".tmpl"))
	}

	sort.Strings(names)
	return names
}

// Load reads and parses a template file.
func Load(path string, opts ...Option) (*Template, error) {
	if path == "" {
		return nil, errors.New("template path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return ParseTemplate(filepath.Base(path), string(data), opts...)
}

// LoadFromFS reads and parses a template from an fs.FS.
func LoadFromFS(tmplFS fs.FS, path string, opts ...Option) (*Template, error) {
	if tmplFS == nil {
		return nil, errors.New("template filesystem is required")
	}
	if path == "" {
		return nil, errors.New("template path is required")
	}
	file, err := tmplFS.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return ParseTemplate(path, string(data), opts...)
}

// LoadSource reads and parses a template from a file path or builtin reference.
func LoadSource(templateFS fs.FS, source string, opts ...Option) (*Template, error) {
	parsed, err := ParseSource(templateFS, source, opts...)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: parsed}, nil
}

// ReadSource reads a template from a file path or builtin reference.
func ReadSource(templateFS fs.FS, source string) (name string, body string, err error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", errors.New("template source is required")
	}
	if IsBuiltin(source) {
		return New(templateFS).Read(source)
	}

	bodyBytes, err := os.ReadFile(source)
	if err != nil {
		return "", "", fmt.Errorf("read template %q: %w", source, err)
	}
	return filepath.Base(source), string(bodyBytes), nil
}

// ParseSource reads and parses a template from a file path or builtin reference.
func ParseSource(templateFS fs.FS, source string, opts ...Option) (*template.Template, error) {
	name, body, err := ReadSource(templateFS, source)
	if err != nil {
		return nil, err
	}

	parsed, err := Parse(name, body, opts...)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", source, err)
	}
	return parsed, nil
}

// Parse parses one template string with the shared function map.
func Parse(name, value string, opts ...Option) (*template.Template, error) {
	cfg, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}

	funcs := FuncMap()
	maps.Copy(funcs, cfg.funcs)

	parsed, err := template.New(name).Option(cfg.templateOption()).Funcs(funcs).Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

// ParseTemplate parses a byte-rendering template from a string.
func ParseTemplate(name, input string, opts ...Option) (*Template, error) {
	if input == "" {
		return nil, errors.New("template input is required")
	}
	parsed, err := Parse(name, input, opts...)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: parsed}, nil
}

// ParseStringTemplate parses a string-rendering template from a string.
func ParseStringTemplate(name, input string, opts ...Option) (*StringTemplate, error) {
	if input == "" {
		return nil, errors.New("template input is required")
	}
	parsed, err := Parse(name, input, opts...)
	if err != nil {
		return nil, err
	}
	return &StringTemplate{tmpl: parsed}, nil
}

// LoadString reads a file and parses it into a StringTemplate.
func LoadString(path string, opts ...Option) (*StringTemplate, error) {
	if path == "" {
		return nil, errors.New("template path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return ParseStringTemplate(filepath.Base(path), string(data), opts...)
}

// LoadStringFromFS reads a file from an fs.FS into a StringTemplate.
func LoadStringFromFS(tmplFS fs.FS, path string, opts ...Option) (*StringTemplate, error) {
	if tmplFS == nil {
		return nil, errors.New("template filesystem is required")
	}
	if path == "" {
		return nil, errors.New("template path is required")
	}
	file, err := tmplFS.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return ParseStringTemplate(path, string(data), opts...)
}

// Execute renders tmpl with data and returns the rendered string.
func Execute(tmpl *template.Template, data any) (string, error) {
	if tmpl == nil {
		return "", errors.New("template is nil")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// Render executes the template with the provided data and returns bytes.
func (t *Template) Render(data any) ([]byte, error) {
	if t == nil || t.tmpl == nil {
		return nil, errors.New("template is nil")
	}
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// Render executes the template with the provided data and returns a string.
func (t *StringTemplate) Render(data any) (string, error) {
	if t == nil || t.tmpl == nil {
		return "", errors.New("template is nil")
	}
	return Execute(t.tmpl, data)
}

// parseOptions applies template parse options and validates defaults.
func parseOptions(opts ...Option) (options, error) {
	cfg := options{missingKey: MissingKeyError}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.missingKey == "" {
		cfg.missingKey = MissingKeyError
	}
	if err := validateMissingKey(cfg.missingKey); err != nil {
		return options{}, err
	}
	return cfg, nil
}

// validateMissingKey reports invalid text/template missingkey policies.
func validateMissingKey(policy MissingKey) error {
	switch policy {
	case MissingKeyDefault, MissingKeyZero, MissingKeyError:
		return nil
	default:
		return fmt.Errorf("invalid missing key policy %q", policy)
	}
}

// templateOption returns the text/template option string for missing keys.
func (o options) templateOption() string {
	return "missingkey=" + string(o.missingKey)
}

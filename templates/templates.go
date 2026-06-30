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
	// builtinPrefix marks a template source as an embedded built-in template.
	builtinPrefix = "builtin:"
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
	missingKey      MissingKey
	funcs           template.FuncMap
	useDefaultFuncs bool
}

// WithMissingKey configures the text/template missingkey option.
func WithMissingKey(policy MissingKey) Option {
	return func(cfg *options) {
		cfg.missingKey = policy
	}
}

// WithDefaultFuncs enables Notifykit's default template helper functions.
func WithDefaultFuncs() Option {
	return func(cfg *options) {
		cfg.useDefaultFuncs = true
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

// builtinTemplates reads built-in templates from an injected filesystem.
type builtinTemplates struct {
	files fs.FS
}

// newBuiltins creates a built-in template registry backed by files.
//
// The returned registry is used for builtin: template sources. A nil filesystem
// is allowed, but read and exists will return an error and names will return nil.
func newBuiltins(files fs.FS) builtinTemplates {
	return builtinTemplates{files: files}
}

// DefaultFuncs returns Notifykit's default template helper functions.
//
// The returned map is a fresh copy and can be modified by callers.
func DefaultFuncs() template.FuncMap {
	return tmplfuncs.FuncMap()
}

// isBuiltin reports whether source references an embedded built-in template.
func isBuiltin(source string) bool {
	return strings.HasPrefix(strings.TrimSpace(source), builtinPrefix)
}

// builtinName trims the builtin prefix and surrounding whitespace from source.
func builtinName(source string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(source), builtinPrefix))
}

// read returns an embedded built-in template by name.
func (t builtinTemplates) read(source string) (filename string, body string, err error) {
	name := builtinName(source)
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

// exists reports whether the named built-in template can be read.
func (t builtinTemplates) exists(source string) error {
	_, _, err := t.read(source)
	return err
}

// names returns all available built-in template names.
func (t builtinTemplates) names() []string {
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
	parsed, err := parseSource(templateFS, source, opts...)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: parsed}, nil
}

// readSource reads a template from a file path or builtin reference.
func readSource(templateFS fs.FS, source string) (name string, body string, err error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", errors.New("template source is required")
	}
	if isBuiltin(source) {
		return newBuiltins(templateFS).read(source)
	}

	bodyBytes, err := os.ReadFile(source)
	if err != nil {
		return "", "", fmt.Errorf("read template %q: %w", source, err)
	}
	return filepath.Base(source), string(bodyBytes), nil
}

// parseSource reads and parses a template from a file path or builtin reference.
func parseSource(templateFS fs.FS, source string, opts ...Option) (*template.Template, error) {
	name, body, err := readSource(templateFS, source)
	if err != nil {
		return nil, err
	}

	parsed, err := parse(name, body, opts...)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", source, err)
	}
	return parsed, nil
}

// parse parses one template string with the shared function map.
func parse(name, value string, opts ...Option) (*template.Template, error) {
	cfg, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}

	funcs := template.FuncMap{}
	if cfg.useDefaultFuncs {
		maps.Copy(funcs, DefaultFuncs())
	}
	maps.Copy(funcs, cfg.funcs)

	tmpl := template.New(name).
		Option(cfg.templateOption()).
		Funcs(funcs)

	parsed, err := tmpl.Parse(value)
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
	parsed, err := parse(name, input, opts...)
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
	parsed, err := parse(name, input, opts...)
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

// execute renders tmpl with data and returns the rendered string.
func execute(tmpl *template.Template, data any) (string, error) {
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
	return execute(t.tmpl, data)
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

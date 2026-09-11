package templates

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
)

// Renderer renders a notification body. Implementations must support concurrent calls.
type Renderer interface{ Render(any) ([]byte, error) }

// HTMLTemplate renders HTML with contextual escaping. Template source and custom
// functions must be trusted; use ordinary strings for untrusted notification data.
type HTMLTemplate struct{ tmpl *template.Template }

// ParseHTMLTemplate parses an HTML body with the same options as text templates.
func ParseHTMLTemplate(name, input string, opts ...Option) (*HTMLTemplate, error) {
	if input == "" {
		return nil, errors.New("template input is required")
	}
	cfg, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{}
	if cfg.useDefaultFuncs {
		maps.Copy(funcs, DefaultFuncs())
	}
	maps.Copy(funcs, cfg.funcs)
	parsed, err := template.New(name).Option(cfg.templateOption()).Funcs(funcs).Parse(input)
	if err != nil {
		return nil, fmt.Errorf("parse HTML %s: %w", name, err)
	}
	return &HTMLTemplate{tmpl: parsed}, nil
}

// LoadHTMLSource loads an HTML template from a file or builtin reference.
func LoadHTMLSource(files fs.FS, source string, opts ...Option) (*HTMLTemplate, error) {
	name, body, err := readSource(files, source)
	if err != nil {
		return nil, err
	}
	return ParseHTMLTemplate(name, body, opts...)
}
func (t *HTMLTemplate) Render(data any) ([]byte, error) {
	if t == nil || t.tmpl == nil {
		return nil, errors.New("HTML template is nil")
	}
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute HTML template: %w", err)
	}
	return buf.Bytes(), nil
}

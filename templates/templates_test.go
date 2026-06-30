package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithMissingKey tests expected behavior.
func TestWithMissingKey(t *testing.T) {
	t.Parallel()

	cfg := options{}
	WithMissingKey(MissingKeyDefault)(&cfg)
	assert.Equal(t, MissingKeyDefault, cfg.missingKey)
}

// TestWithFunc tests expected behavior.
func TestWithFunc(t *testing.T) {
	t.Parallel()

	cfg := options{}
	WithFunc("upper", strings.ToUpper)(&cfg)

	assert.Contains(t, cfg.funcs, "upper")
}

// TestWithFuncs tests expected behavior.
func TestWithFuncs(t *testing.T) {
	t.Parallel()

	funcs := template.FuncMap{"upper": strings.ToUpper}
	cfg := options{}

	WithFuncs(funcs)(&cfg)
	funcs["lower"] = strings.ToLower

	assert.Contains(t, cfg.funcs, "upper")
	assert.NotContains(t, cfg.funcs, "lower")
}

// TestNewBuiltins tests expected behavior.
func TestNewBuiltins(t *testing.T) {
	t.Parallel()

	files := fstest.MapFS{"hello.tmpl": {Data: []byte("hello")}}
	registry := newBuiltins(files)
	assert.NotNil(t, registry.files)
}

// TestFuncMap tests expected behavior.
func TestFuncMap(t *testing.T) {
	t.Parallel()

	funcs := funcMap()

	assert.NotEmpty(t, funcs)
	assert.Contains(t, funcs, "json")
	assert.Contains(t, funcs, "default")
	assert.Contains(t, funcs, "withPrefix")
	assert.Contains(t, funcs, "optional")
	assert.Contains(t, funcs, "when")

	assert.Contains(t, funcs, "coalesce")
	assert.Contains(t, funcs, "formatTime")
	assert.Contains(t, funcs, "trim")
	assert.Contains(t, funcs, "upper")
	assert.Contains(t, funcs, "lower")
	assert.Contains(t, funcs, "withSuffix")
	assert.Contains(t, funcs, "duration")
}

// TestIsBuiltin tests expected behavior.
func TestIsBuiltin(t *testing.T) {
	t.Parallel()

	t.Run("matches builtin prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, isBuiltin(" builtin:slack "))
	})

	t.Run("rejects file path", func(t *testing.T) {
		t.Parallel()

		assert.False(t, isBuiltin("slack.tmpl"))
	})
}

// TestBuiltinName tests expected behavior.
func TestBuiltinName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "slack", builtinName(" builtin:slack "))
}

// TestBuiltinTemplatesRead tests expected behavior.
func TestBuiltinTemplatesRead(t *testing.T) {
	t.Parallel()

	t.Run("reads builtin template", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{"hello.tmpl": {Data: []byte("hello")}})
		name, body, err := registry.read("builtin:hello")
		require.NoError(t, err)
		assert.Equal(t, "hello.tmpl", name)
		assert.Equal(t, "hello", body)
	})

	t.Run("requires name", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{})
		_, _, err := registry.read("builtin:")
		require.Error(t, err)
	})

	t.Run("rejects path separators", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{})
		_, _, err := registry.read("builtin:../secret")
		require.Error(t, err)
	})

	t.Run("requires configured filesystem", func(t *testing.T) {
		t.Parallel()

		_, _, err := newBuiltins(nil).read("builtin:hello")
		require.Error(t, err)
	})

	t.Run("wraps read errors", func(t *testing.T) {
		t.Parallel()

		_, _, err := newBuiltins(fstest.MapFS{}).read("builtin:missing")
		require.Error(t, err)
	})
}

// TestBuiltinTemplatesExists tests expected behavior.
func TestBuiltinTemplatesExists(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when template exists", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{"hello.tmpl": {Data: []byte("hello")}})
		err := registry.exists("builtin:hello")
		require.NoError(t, err)
	})

	t.Run("returns read error", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{})
		err := registry.exists("builtin:missing")
		require.Error(t, err)
	})
}

// TestBuiltinTemplatesNames tests expected behavior.
func TestBuiltinTemplatesNames(t *testing.T) {
	t.Parallel()

	t.Run("returns sorted template names", func(t *testing.T) {
		t.Parallel()

		registry := newBuiltins(fstest.MapFS{
			"z.tmpl":     {Data: []byte("z")},
			"a.tmpl":     {Data: []byte("a")},
			"skip.txt":   {Data: []byte("skip")},
			"dir/x.tmpl": {Data: []byte("x")},
		})
		assert.Equal(t, []string{"a", "z"}, registry.names())
	})

	t.Run("returns nil without filesystem", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, newBuiltins(nil).names())
	})
}

// TestLoad tests expected behavior.
func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("loads file template", func(t *testing.T) {
		t.Parallel()

		path := writeTempTemplate(t, "hello {{ .Name }}")
		tmpl, err := Load(path)
		require.NoError(t, err)
		out, err := tmpl.Render(map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(out))
	})

	t.Run("requires path", func(t *testing.T) {
		t.Parallel()

		tmpl, err := Load("")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})

	t.Run("returns read error", func(t *testing.T) {
		t.Parallel()

		tmpl, err := Load(filepath.Join(t.TempDir(), "missing.tmpl"))
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestLoadFromFS tests expected behavior.
func TestLoadFromFS(t *testing.T) {
	t.Parallel()

	t.Run("loads template from filesystem", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadFromFS(fstest.MapFS{"hello.tmpl": {Data: []byte("hello {{ .Name }}")}}, "hello.tmpl")
		require.NoError(t, err)
		out, err := tmpl.Render(map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(out))
	})

	t.Run("requires path", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadFromFS(fstest.MapFS{}, "")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})

	t.Run("returns open error", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadFromFS(fstest.MapFS{}, "missing.tmpl")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestLoadSource tests expected behavior.
func TestLoadSource(t *testing.T) {
	t.Parallel()

	t.Run("loads builtin source", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadSource(fstest.MapFS{"hello.tmpl": {Data: []byte("hello")}}, "builtin:hello")
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(out))
	})

	t.Run("loads file source", func(t *testing.T) {
		t.Parallel()

		path := writeTempTemplate(t, "hello")
		tmpl, err := LoadSource(nil, path)
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(out))
	})
}

// TestReadSource tests expected behavior.
func TestReadSource(t *testing.T) {
	t.Parallel()

	t.Run("reads builtin source", func(t *testing.T) {
		t.Parallel()

		name, body, err := readSource(fstest.MapFS{"hello.tmpl": {Data: []byte("hello")}}, "builtin:hello")
		require.NoError(t, err)
		assert.Equal(t, "hello.tmpl", name)
		assert.Equal(t, "hello", body)
	})

	t.Run("reads file source", func(t *testing.T) {
		t.Parallel()

		path := writeTempTemplate(t, "hello")
		name, body, err := readSource(nil, path)
		require.NoError(t, err)
		assert.Equal(t, filepath.Base(path), name)
		assert.Equal(t, "hello", body)
	})

	t.Run("requires source", func(t *testing.T) {
		t.Parallel()

		_, _, err := readSource(nil, "")
		require.Error(t, err)
	})
}

// TestParseSource tests expected behavior.
func TestParseSource(t *testing.T) {
	t.Parallel()

	t.Run("parses source", func(t *testing.T) {
		t.Parallel()

		parsed, err := parseSource(fstest.MapFS{"hello.tmpl": {Data: []byte("hello {{ .Name }}")}}, "builtin:hello")
		require.NoError(t, err)
		out, err := execute(parsed, map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", out)
	})

	t.Run("wraps parse error", func(t *testing.T) {
		t.Parallel()

		parsed, err := parseSource(
			fstest.MapFS{
				"bad.tmpl": {Data: []byte("{{")},
			},
			"builtin:bad",
		)
		require.Error(t, err)
		assert.Nil(t, parsed)
	})
}

// TestParse tests expected behavior.
func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("parses template", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", "hello {{ .Name }}")
		require.NoError(t, err)
		out, err := execute(parsed, map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", out)
	})

	t.Run("uses strict missing key by default", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", "{{ .Missing }}")
		require.NoError(t, err)
		_, err = execute(parsed, map[string]any{})
		require.Error(t, err)
	})

	t.Run("allows default missing key behavior", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", "{{ .Missing }}", WithMissingKey(MissingKeyDefault))
		require.NoError(t, err)
		out, err := execute(parsed, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "<no value>", out)
	})

	t.Run("uses default helper functions", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", `{{ .Channel | default "alertmanager" | withPrefix "#" }} {{ when .Resolved "up" "down" }} {{ optional "%s: %s" .Label .Value }}`)
		require.NoError(t, err)

		out, err := execute(parsed, map[string]any{
			"Channel":  "",
			"Resolved": true,
			"Label":    "Status",
			"Value":    "ok",
		})
		require.NoError(t, err)
		assert.Equal(t, "#alertmanager up Status: ok", out)
	})

	t.Run("uses custom function", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", `{{ "notifykit" | upper }}`, WithFunc("upper", strings.ToUpper))
		require.NoError(t, err)

		out, err := execute(parsed, nil)
		require.NoError(t, err)
		assert.Equal(t, "NOTIFYKIT", out)
	})

	t.Run("uses custom function map", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", `{{ "notifykit" | wrap }}`, WithFuncs(template.FuncMap{
			"wrap": func(value string) string { return "[" + value + "]" },
		}))
		require.NoError(t, err)

		out, err := execute(parsed, nil)
		require.NoError(t, err)
		assert.Equal(t, "[notifykit]", out)
	})

	t.Run("uses tmplfuncs helpers by default", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", `{{ .Duration | duration }}`)
		require.NoError(t, err)

		out, err := execute(parsed, map[string]any{"Duration": 90 * time.Second})
		require.NoError(t, err)
		assert.Equal(t, "1m30s", out)
	})

	t.Run("rejects invalid missing key policy", func(t *testing.T) {
		t.Parallel()

		parsed, err := parse("hello", "hello", WithMissingKey(MissingKey("bad")))
		require.Error(t, err)
		assert.Nil(t, parsed)
	})
}

// TestParseTemplate tests expected behavior.
func TestParseTemplate(t *testing.T) {
	t.Parallel()

	t.Run("parses byte template", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseTemplate("hello", "hello")
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(out))
	})

	t.Run("requires input", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseTemplate("hello", "")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestParseStringTemplate tests expected behavior.
func TestParseStringTemplate(t *testing.T) {
	t.Parallel()

	t.Run("parses string template", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseStringTemplate("hello", "hello")
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	})

	t.Run("requires input", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseStringTemplate("hello", "")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestLoadString tests expected behavior.
func TestLoadString(t *testing.T) {
	t.Parallel()

	t.Run("loads string template from file", func(t *testing.T) {
		t.Parallel()

		path := writeTempTemplate(t, "hello {{ .Name }}")
		tmpl, err := LoadString(path)
		require.NoError(t, err)
		out, err := tmpl.Render(map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", out)
	})

	t.Run("requires path", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadString("")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestLoadStringFromFS tests expected behavior.
func TestLoadStringFromFS(t *testing.T) {
	t.Parallel()

	t.Run("loads string template from filesystem", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadStringFromFS(fstest.MapFS{"hello.tmpl": {Data: []byte("hello {{ .Name }}")}}, "hello.tmpl")
		require.NoError(t, err)
		out, err := tmpl.Render(map[string]any{"Name": "world"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", out)
	})

	t.Run("requires path", func(t *testing.T) {
		t.Parallel()

		tmpl, err := LoadStringFromFS(fstest.MapFS{}, "")
		require.Error(t, err)
		assert.Nil(t, tmpl)
	})
}

// TestExecute tests expected behavior.
func TestExecute(t *testing.T) {
	t.Parallel()

	t.Run("executes template", func(t *testing.T) {
		t.Parallel()

		parsed := template.Must(template.New("hello").Parse("hello"))
		out, err := execute(parsed, nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	})

	t.Run("requires template", func(t *testing.T) {
		t.Parallel()

		out, err := execute(nil, nil)
		require.Error(t, err)
		assert.Empty(t, out)
	})
}

// TestTemplateRender tests expected behavior.
func TestTemplateRender(t *testing.T) {
	t.Parallel()

	t.Run("renders bytes", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseTemplate("hello", "hello")
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), out)
	})

	t.Run("requires template", func(t *testing.T) {
		t.Parallel()

		var tmpl *Template
		out, err := tmpl.Render(nil)
		require.Error(t, err)
		assert.Nil(t, out)
	})
}

// TestStringTemplateRender tests expected behavior.
func TestStringTemplateRender(t *testing.T) {
	t.Parallel()

	t.Run("renders string", func(t *testing.T) {
		t.Parallel()

		tmpl, err := ParseStringTemplate("hello", "hello")
		require.NoError(t, err)
		out, err := tmpl.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	})

	t.Run("requires template", func(t *testing.T) {
		t.Parallel()

		var tmpl *StringTemplate
		out, err := tmpl.Render(nil)
		require.Error(t, err)
		assert.Empty(t, out)
	})
}

// TestParseOptions tests expected behavior.
func TestParseOptions(t *testing.T) {
	t.Parallel()

	t.Run("defaults to missing key error", func(t *testing.T) {
		t.Parallel()

		cfg, err := parseOptions()
		require.NoError(t, err)
		assert.Equal(t, MissingKeyError, cfg.missingKey)
	})

	t.Run("ignores nil options", func(t *testing.T) {
		t.Parallel()

		cfg, err := parseOptions(nil)
		require.NoError(t, err)
		assert.Equal(t, MissingKeyError, cfg.missingKey)
	})
}

// TestValidateMissingKey tests expected behavior.
func TestValidateMissingKey(t *testing.T) {
	t.Parallel()

	t.Run("accepts known policy", func(t *testing.T) {
		t.Parallel()

		err := validateMissingKey(MissingKeyZero)
		require.NoError(t, err)
	})

	t.Run("rejects unknown policy", func(t *testing.T) {
		t.Parallel()

		err := validateMissingKey(MissingKey("bad"))
		require.Error(t, err)
	})
}

// TestOptionsTemplateOption tests expected behavior.
func TestOptionsTemplateOption(t *testing.T) {
	t.Parallel()

	cfg := options{missingKey: MissingKeyZero}
	assert.Equal(t, "missingkey=zero", cfg.templateOption())
}

// writeTempTemplate supports tests.
func writeTempTemplate(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.tmpl")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

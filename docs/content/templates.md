# Templates

Text templates use Go `text/template`. HTML templates use `html/template` for contextual escaping. No helper functions are enabled by default; opt in to Notifykit's default helpers with `WithDefaultFuncs`.

```go
subject, err := templates.ParseStringTemplate("subject", `{{ .Service }} is {{ .Status }}`)
body, err := templates.LoadSource(templateFS, "builtin:slack", templates.WithDefaultFuncs())
```

Missing map keys fail by default. Use `WithMissingKey` for looser templates:

```go
tmpl, err := templates.ParseStringTemplate(
    "subject",
    `{{ .Service }}`,
    templates.WithMissingKey(templates.MissingKeyDefault),
)
```

`WithDefaultFuncs` enables the helper map returned by `DefaultFuncs`. Notifykit uses `github.com/containeroo/tmplfuncs` for these helpers, including `json`, `default`, `coalesce`, `formatTime`, `trim`, `upper`, `lower`, `withPrefix`, `withSuffix`, `optional`, `when`, and `duration`:

```go
body, err := templates.ParseTemplate(
    "webhook",
    `{"text": {{ print
        "Expected every: " (.ExpectedEvery | duration) "\n"
        "Expected by: " (.ExpectedBy | formatTime "2006-01-02 15:04:05 MST")
        | json
    }}}`,
    templates.WithDefaultFuncs(),
)
```

Applications can add or override project-specific template functions with `WithFunc` or `WithFuncs`.
This is useful for formatting that should be owned by the application.

```go
func formatDuration(d time.Duration) string {
    if d < 0 {
        d = -d
    }
    if d < time.Second {
        return d.Truncate(time.Millisecond).String()
    }
    return d.Truncate(time.Second).String()
}

body, err := templates.ParseTemplate(
    "webhook",
    `{"text": {{ (.Duration | formatDuration) | json }}}`,
    templates.WithDefaultFuncs(),
    templates.WithFunc("formatDuration", formatDuration),
)
```

For complete control, build a function map yourself and pass only the helpers you want:

```go
funcs := templates.DefaultFuncs()
delete(funcs, "duration")
funcs["formatDuration"] = formatDuration

body, err := templates.ParseTemplate(
    "webhook",
    `{"text": {{ (.Duration | formatDuration) | json }}}`,
    templates.WithFuncs(funcs),
)
```

## HTML email

```go
body, err := templates.ParseHTMLTemplate(
    "email",
    `<h1>{{ .Subject }}</h1><p>{{ .Message }}</p><a href="{{ .URL }}">Details</a>`,
)
```

Dynamic strings are escaped for their HTML context, including attributes and
URLs. Existing text templates do not do this. Keep template source and custom
functions trusted. Pass untrusted notification data as ordinary strings, not as
`template.HTML` or other trusted HTML types.

Load an HTML file or injected builtin with `LoadHTMLSource`:

```go
body, err := templates.LoadHTMLSource(nil, "templates/email.tmpl", templates.WithDefaultFuncs())
```

## Loading templates

| Function | Input | Output |
| --- | --- | --- |
| `ParseTemplate` | Text string | Byte renderer |
| `ParseStringTemplate` | Text string | String renderer |
| `ParseHTMLTemplate` | HTML string | Escaped byte renderer |
| `Load` / `LoadString` | Filesystem path | Text renderer |
| `LoadFromFS` / `LoadStringFromFS` | `fs.FS` and path | Text renderer |
| `LoadSource` | File path or `builtin:name` | Text byte renderer |
| `LoadHTMLSource` | File path or `builtin:name` | Escaped HTML renderer |

A `builtin:name` resolves `name.tmpl` at the root of the supplied filesystem.
Applications supply that filesystem, for example through `embed.FS` and `fs.Sub`;
Notifykit does not ship an application-specific builtin catalog. Names cannot
contain path separators. File paths passed to `LoadSource` and `LoadHTMLSource`
are read from the OS filesystem.

## Two render passes

Targets call `payload.Data("")` to render their title or subject. They then call
`payload.Data(renderedTitleOrSubject)` to render the body. Your notification
chooses the data shape: `.Title` and `.Subject` are conventions, not fields
injected automatically by Notifykit.

Parsed templates can render concurrently. Any custom functions and data they
reference must also support concurrent use. Missing-key policy is shared by text
and HTML parsing.

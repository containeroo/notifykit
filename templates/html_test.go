package templates

import (
	"github.com/stretchr/testify/require"
	"testing"
	"testing/fstest"
)

func TestHTMLContextEscaping(t *testing.T) {
	tmpl, err := ParseHTMLTemplate("mail", `<p>{{.Name}}</p><a href="{{.URL}}">link</a>`)
	require.NoError(t, err)
	output, err := tmpl.Render(map[string]any{"Name": "<img src=x>", "URL": "javascript:alert(1)"})
	require.NoError(t, err)
	require.Contains(t, string(output), "&lt;img src=x&gt;")
	require.Contains(t, string(output), "#ZgotmplZ")
	_, err = tmpl.Render(map[string]any{})
	require.Error(t, err)
	loaded, err := LoadHTMLSource(fstest.MapFS{"mail.tmpl": {Data: []byte(`{{.Name | upper}}`)}}, "builtin:mail", WithDefaultFuncs())
	require.NoError(t, err)
	output, err = loaded.Render(map[string]any{"Name": "hello"})
	require.NoError(t, err)
	require.Equal(t, "HELLO", string(output))
}

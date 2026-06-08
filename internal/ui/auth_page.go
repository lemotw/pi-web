package ui

import (
	"bytes"
	_ "embed"
	"html/template"
	"io"
)

//go:embed embedded/auth.html
var authTmplStr string

var authTmpl = template.Must(template.New("auth").Parse(authTmplStr))

// RenderAuthPrompt writes the standalone token-gate page. It reuses the shared
// theme stylesheet and boot script so the login screen honors every built-in
// theme instead of a hardcoded dark/light copy that drifts from theme.css. The
// runtime "custom" theme is unreachable here because /custom-themes.css is
// itself behind auth, so only the inlined built-ins apply.
func RenderAuthPrompt(w io.Writer) error {
	data := struct {
		ThemeCss  template.HTML
		ThemeBoot template.HTML
	}{
		ThemeCss:  template.HTML("<style>\n" + liveThemeCss + "\n</style>"),
		ThemeBoot: liveThemeBootScript(),
	}
	var buf bytes.Buffer
	if err := authTmpl.Execute(&buf, data); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

package ui

import (
	"embed"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"pi-web/internal/sessions"
)

//go:embed export/index.html
var exportHtml string

//go:embed export/template.css
var exportSessionCss string

//go:embed export/app/*.js
var appJsFS embed.FS

//go:embed export/vendor/marked.min.js
var markedJs string

//go:embed export/vendor/highlight.min.js
var hljsJs string

// Explicit export app JS manifest. The static export runtime is intentionally
// not bundled by Vite, so keep load order here instead of relying on filename
// sorting to silently do the right thing.
var exportAppJSFiles = []string{
	"00-data.js",
	"10-tree.js",
	"20-filter.js",
	"30-format.js",
	"40-render-tree.js",
	"50-render-entry.js",
	"60-header.js",
	"70-navigation.js",
	"80-ui.js",
}

// Concatenated export app JS bundle, wrapped in a single IIFE so all modules
// share closure scope.
var exportJs = buildExportJsBundle()

func buildExportJsBundle() string {
	verifyExportAppManifest()

	var b strings.Builder
	b.WriteString("(function() {\n'use strict';\n")
	for _, name := range exportAppJSFiles {
		body, err := appJsFS.ReadFile("export/app/" + name)
		if err != nil {
			panic(fmt.Errorf("read %s: %w", name, err))
		}
		b.WriteString("\n// ===== " + name + " =====\n")
		b.Write(body)
		b.WriteString("\n")
	}
	b.WriteString("})();\n")
	return b.String()
}

func verifyExportAppManifest() {
	entries, err := appJsFS.ReadDir("export/app")
	if err != nil {
		panic(fmt.Errorf("read export/app: %w", err))
	}

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			seen[e.Name()] = true
		}
	}
	for _, name := range exportAppJSFiles {
		if !seen[name] {
			panic(fmt.Errorf("export app manifest references missing file %s", name))
		}
		delete(seen, name)
	}
	if len(seen) > 0 {
		extra := make([]string, 0, len(seen))
		for name := range seen {
			extra = append(extra, name)
		}
		sort.Strings(extra)
		panic(fmt.Errorf("export app files missing from manifest: %s", strings.Join(extra, ", ")))
	}
}

// renderExportSessionPage renders a self-contained HTML snapshot suitable for
// GitHub Gist sharing. All JS is inlined and server-dependent chrome (buttons,
// chat composer) is stripped.
func RenderExportSessionPage(session sessions.Session) string {
	dataBase64, css, bodyAttrs := prepareSessionPageData(session, exportSessionCss)

	html := exportHtml
	html = replaceRequired(html, "{{TITLE}}", template.HTMLEscapeString(session.Name))
	html = replaceRequired(html, "{{CSS}}", css)
	html = replaceRequired(html, "{{BODY_ATTRS}}", bodyAttrs)
	html = replaceRequired(html, "{{SESSION_DATA}}", dataBase64)

	inlineScript := "<script>\n" + markedJs + "\n</script>\n<script>\n" + hljsJs + "\n</script>\n<script>\n" + exportJs + "\n</script>"
	html = replaceRequired(html, "{{SESSION_SCRIPT}}", inlineScript)
	html = replaceRequired(html, "{{CHAT_COMPOSER}}", "")

	return html
}

func renderExportSessionPage(session sessions.Session) string {
	return RenderExportSessionPage(session)
}

func serveStaticJS(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write([]byte(body))
	}
}

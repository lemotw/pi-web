package main

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

//go:embed export/vendor/alpine.min.js
var alpineJs string

// Concatenated export app JS bundle, wrapped in a single IIFE so all modules
// share closure scope. Files are concatenated in lexical order — the numeric
// prefix (00-data.js, 10-tree.js, ...) controls evaluation order.
var exportJs = buildExportJsBundle()

func buildExportJsBundle() string {
	entries, err := appJsFS.ReadDir("export/app")
	if err != nil {
		panic(fmt.Errorf("read export/app: %w", err))
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("(function() {\n'use strict';\n")
	for _, name := range names {
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

// renderExportSessionPage renders a self-contained HTML snapshot suitable for
// GitHub Gist sharing. All JS is inlined and server-dependent chrome (buttons,
// chat composer) is stripped.
func renderExportSessionPage(session sessions.Session) string {
	dataBase64, css, bodyAttrs := prepareSessionPageData(session, exportSessionCss)

	html := exportHtml
	html = strings.Replace(html, "{{TITLE}}", template.HTMLEscapeString(session.Name), 1)
	html = strings.Replace(html, "{{CSS}}", css, 1)
	html = strings.Replace(html, "{{BODY_ATTRS}}", bodyAttrs, 1)
	html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)

	inlineScript := "<script>\n" + markedJs + "\n</script>\n<script>\n" + hljsJs + "\n</script>\n<script>\n" + exportJs + "\n</script>"
	html = strings.Replace(html, "{{SESSION_SCRIPT}}", inlineScript, 1)
	html = strings.Replace(html, "{{CHAT_COMPOSER}}", "", 1)

	return html
}

func serveStaticJS(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write([]byte(body))
	}
}

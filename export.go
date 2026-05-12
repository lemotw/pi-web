package main

import (
	"embed"
	_ "embed"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

//go:embed export/app/*.js
var appJsFS embed.FS

//go:embed export/vendor/marked.min.js
var markedJs string

//go:embed export/vendor/highlight.min.js
var hljsJs string

//go:embed export/vendor/alpine.min.js
var alpineJs string

// Concatenated app JS bundle, wrapped in a single IIFE so all modules share
// closure scope. Files are concatenated in lexical order — the numeric prefix
// (00-data.js, 10-tree.js, ...) controls evaluation order.
var templateJs = buildTemplateJsBundle()

func buildTemplateJsBundle() string {
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

func serveStaticJS(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write([]byte(body))
	}
}

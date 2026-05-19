package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"time"
)

//go:embed live_templates/index.html
var indexTmplStr string

func fmtTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func fmtCost(n float64) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.4f", n)
}

func fmtRelativeTime(ts string) string {
	if ts == "" {
		return "—"
	}
	then, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	seconds := int(time.Since(then).Seconds())
	if seconds < 60 {
		return "just now"
	}
	units := []struct {
		name    string
		seconds int
	}{
		{"year", 365 * 24 * 60 * 60},
		{"month", 30 * 24 * 60 * 60},
		{"week", 7 * 24 * 60 * 60},
		{"day", 24 * 60 * 60},
		{"hour", 60 * 60},
		{"minute", 60},
	}
	for _, unit := range units {
		if count := seconds / unit.seconds; count >= 1 {
			if count == 1 {
				return "1 " + unit.name + " ago"
			}
			return fmt.Sprintf("%d %ss ago", count, unit.name)
		}
	}
	return "just now"
}

var funcMap = template.FuncMap{
	"fmtTokens":       fmtTokens,
	"fmtCost":         fmtCost,
	"fmtRelativeTime": fmtRelativeTime,
	"indexScript":     func() string { return indexScriptPath },
	"indexPreload": func() template.HTML {
		return template.HTML(`<link rel="modulepreload" href="` + template.HTMLEscapeString(indexScriptPath) + `">`)
	},
}

var indexTmpl = template.Must(template.New("index").Funcs(funcMap).Parse(indexTmplStr))

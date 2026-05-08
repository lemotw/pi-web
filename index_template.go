package main

import (
	_ "embed"
	"fmt"
	"html/template"
)

//go:embed templates/index.html
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

var funcMap = template.FuncMap{
	"fmtTokens":   fmtTokens,
	"fmtCost":     fmtCost,
	"indexScript": func() string { return indexScriptPath },
}

var indexTmpl = template.Must(template.New("index").Funcs(funcMap).Parse(indexTmplStr))

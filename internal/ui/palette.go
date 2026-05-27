package ui

import (
	_ "embed"
	"html/template"
	"strings"
)

//go:embed live_templates/palette.html
var paletteTmplStr string

var paletteTmpl = template.Must(template.New("palette").Parse(paletteTmplStr))

type paletteData struct {
	ID       string
	Label    string
	SearchID string
	Actions  bool
}

func renderPalette(data paletteData) template.HTML {
	var b strings.Builder
	if err := paletteTmpl.Execute(&b, data); err != nil {
		panic(err)
	}
	return template.HTML(b.String())
}

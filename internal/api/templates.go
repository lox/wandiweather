package api

import (
	"embed"
	"html/template"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

// newTemplates creates and parses the HTML templates with custom functions.
func newTemplates() *template.Template {
	funcs := template.FuncMap{
		"deref": func(f *float64) float64 {
			if f == nil {
				return 0
			}
			return *f
		},
		"abs": func(f float64) float64 {
			if f < 0 {
				return -f
			}
			return f
		},
		"neg": func(f float64) float64 {
			return -f
		},
		"upper": strings.ToUpper,
	}
	return template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))
}

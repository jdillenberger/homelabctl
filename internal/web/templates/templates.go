package templates

import (
	"embed"
	"fmt"
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

// FS holds all embedded HTML templates.
//
//go:embed *.html
var FS embed.FS

// shared templates that don't define "content" or "title" blocks
var sharedTemplates = []string{"base.html", "stats_partial.html", "stats_compact.html", "alerts_partial.html"}

// Renderer implements echo.Renderer using html/template.
type Renderer struct {
	templates map[string]*template.Template
}

// NewRenderer parses each page template together with the shared templates
// so that each page gets its own "content" and "title" definitions.
func NewRenderer() (*Renderer, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	shared := make(map[string]bool)
	for _, name := range sharedTemplates {
		shared[name] = true
	}

	pages := make(map[string]*template.Template)

	// Register shared templates so they can be rendered directly (e.g. partials)
	for _, name := range sharedTemplates {
		t, err := template.ParseFS(FS, name)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
		pages[name] = t
	}

	// Register page templates, each combined with the shared templates
	for _, entry := range entries {
		name := entry.Name()
		if shared[name] {
			continue
		}
		t, err := template.ParseFS(FS, append(sharedTemplates, name)...)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
		pages[name] = t
	}

	return &Renderer{templates: pages}, nil
}

// Render renders a template by name into the response.
func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	t, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return t.ExecuteTemplate(w, name, data)
}

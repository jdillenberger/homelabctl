package templates

import (
	"embed"
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

// FS holds all embedded HTML templates.
//
//go:embed *.html
var FS embed.FS

// Renderer implements echo.Renderer using html/template.
type Renderer struct {
	templates *template.Template
}

// NewRenderer parses the embedded templates and returns a Renderer.
func NewRenderer() (*Renderer, error) {
	t, err := template.ParseFS(FS, "*.html")
	if err != nil {
		return nil, err
	}
	return &Renderer{templates: t}, nil
}

// Render renders a template by name into the response.
func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

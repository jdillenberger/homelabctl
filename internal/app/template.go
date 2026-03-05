package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/google/uuid"
)

// TemplateRenderer renders Go templates from embedded app templates.
type TemplateRenderer struct {
	registry *Registry
}

// NewTemplateRenderer creates a new renderer backed by the registry.
func NewTemplateRenderer(registry *Registry) *TemplateRenderer {
	return &TemplateRenderer{registry: registry}
}

// RenderFile renders a single template file for the given app with provided values.
func (r *TemplateRenderer) RenderFile(appName, fileName string, values map[string]string) (string, error) {
	tmplPath := appName + "/" + fileName
	data, err := fs.ReadFile(r.registry.FS(), tmplPath)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	return r.renderString(string(data), values)
}

// RenderAllFiles renders all .tmpl files for an app and returns filename->content map.
// The .tmpl suffix is stripped from the output filename.
func (r *TemplateRenderer) RenderAllFiles(appName string, values map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	tmplDir := appName

	err := fs.WalkDir(r.registry.FS(), tmplDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		if !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		data, err := fs.ReadFile(r.registry.FS(), path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		rendered, err := r.renderString(string(data), values)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", path, err)
		}

		// Strip the <app>/ prefix and .tmpl suffix
		relPath := strings.TrimPrefix(path, tmplDir+"/")
		outName := strings.TrimSuffix(relPath, ".tmpl")
		result[outName] = rendered
		return nil
	})

	return result, err
}

// CopyStaticFiles returns non-template files that should be copied as-is.
func (r *TemplateRenderer) CopyStaticFiles(appName string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	tmplDir := appName

	err := fs.WalkDir(r.registry.FS(), tmplDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		// Skip template files and app.yaml
		if strings.HasSuffix(path, ".tmpl") || strings.HasSuffix(path, "app.yaml") {
			return nil
		}

		data, err := fs.ReadFile(r.registry.FS(), path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		relPath := strings.TrimPrefix(path, tmplDir+"/")
		result[relPath] = data
		return nil
	})

	return result, err
}

func (r *TemplateRenderer) renderString(tmplStr string, values map[string]string) (string, error) {
	funcMap := template.FuncMap{
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
		"genPassword": genPassword,
		"genUUID":     func() string { return uuid.New().String() },
		"upper":       strings.ToUpper,
		"lower":       strings.ToLower,
		"replace":     strings.ReplaceAll,
	}

	tmpl, err := template.New("").Option("missingkey=error").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

func genPassword() string {
	return genRandomHex(16)
}

// genLongSecret generates a 128-character hex secret (64 random bytes),
// suitable for Rails secret_key_base and similar high-entropy secrets.
func genLongSecret() string {
	return genRandomHex(64)
}

func genRandomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

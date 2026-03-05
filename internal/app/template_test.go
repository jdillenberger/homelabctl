package app

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestRenderAllFiles(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	renderer := NewTemplateRenderer(r)

	t.Run("renders adguard templates with values", func(t *testing.T) {
		values := map[string]string{
			"web_port": "3000",
			"dns_port": "53",
			"data_dir": "/data/adguard",
		}
		files, err := renderer.RenderAllFiles("adguard", values)
		if err != nil {
			t.Fatalf("RenderAllFiles() error: %v", err)
		}

		if len(files) == 0 {
			t.Fatal("RenderAllFiles() returned no files")
		}

		// Check that docker-compose.yml was rendered (from docker-compose.yml.tmpl)
		compose, ok := files["docker-compose.yml"]
		if !ok {
			t.Fatal("expected docker-compose.yml in rendered output")
		}
		if !strings.Contains(compose, "3000") {
			t.Error("rendered docker-compose.yml should contain port 3000")
		}
		if !strings.Contains(compose, "/data/adguard") {
			t.Error("rendered docker-compose.yml should contain data_dir value")
		}

		// Check .env was rendered
		env, ok := files[".env"]
		if !ok {
			t.Fatal("expected .env in rendered output")
		}
		if !strings.Contains(env, "WEB_PORT=3000") {
			t.Errorf("expected WEB_PORT=3000 in .env, got:\n%s", env)
		}
	})

	t.Run("renders portainer templates", func(t *testing.T) {
		values := map[string]string{
			"http_port":  "9000",
			"https_port": "9443",
			"data_dir":   "/data/portainer",
		}
		files, err := renderer.RenderAllFiles("portainer", values)
		if err != nil {
			t.Fatalf("RenderAllFiles() error: %v", err)
		}

		compose, ok := files["docker-compose.yml"]
		if !ok {
			t.Fatal("expected docker-compose.yml in rendered output")
		}
		if !strings.Contains(compose, "9443") {
			t.Error("rendered docker-compose.yml should contain port 9443")
		}
	})

	t.Run("tmpl suffix stripped from output filenames", func(t *testing.T) {
		values := map[string]string{
			"web_port": "3000",
			"dns_port": "53",
			"data_dir": "/data/adguard",
		}
		files, err := renderer.RenderAllFiles("adguard", values)
		if err != nil {
			t.Fatalf("RenderAllFiles() error: %v", err)
		}

		for name := range files {
			if strings.HasSuffix(name, ".tmpl") {
				t.Errorf("output filename %q still has .tmpl suffix", name)
			}
		}
	})
}

func TestGenPassword(t *testing.T) {
	pw := genPassword()

	if len(pw) != 32 {
		t.Errorf("genPassword() returned %d chars, expected 32", len(pw))
	}

	// Verify it's valid hex
	_, err := hex.DecodeString(pw)
	if err != nil {
		t.Errorf("genPassword() returned non-hex string: %q", pw)
	}

	// Ensure two calls produce different values
	pw2 := genPassword()
	if pw == pw2 {
		t.Error("genPassword() returned identical values on two calls")
	}
}

func TestRenderWithMissingValues(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	renderer := NewTemplateRenderer(r)

	// Render with an empty values map; templates use {{index . "key"}} which
	// returns empty string for missing keys rather than crashing.
	values := map[string]string{}
	files, err := renderer.RenderAllFiles("adguard", values)
	if err != nil {
		t.Fatalf("RenderAllFiles() with empty values should not error, got: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected non-empty file map even with empty values")
	}
}

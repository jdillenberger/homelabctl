package app

import (
	"strings"
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestTraefikTemplateHTTPSDisabled(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewTemplateRenderer(r)
	vals := traefikTestValues()

	files, err := renderer.RenderAllFiles("traefik", vals)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	compose := files["docker-compose.yml"]

	if !strings.Contains(compose, "providers.file.directory=/dynamic") {
		t.Error("should always have file provider for dynamic config")
	}
	if strings.Contains(compose, "/certs") {
		t.Error("HTTPS disabled should NOT mount certs volume")
	}
	if strings.Contains(compose, "certificatesresolvers") {
		t.Error("HTTPS disabled should NOT have ACME config")
	}
}

func TestTraefikTemplateHTTPSNoACME(t *testing.T) {
	r, _ := NewRegistry(templates.FS)
	renderer := NewTemplateRenderer(r)
	vals := traefikTestValues()
	vals["https_enabled"] = "true"
	vals["certs_dir"] = "/opt/homelabctl/data/traefik/certs"

	files, err := renderer.RenderAllFiles("traefik", vals)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	compose := files["docker-compose.yml"]

	if !strings.Contains(compose, "providers.file.directory=/dynamic") {
		t.Error("should have file provider")
	}
	if !strings.Contains(compose, "/certs:ro") {
		t.Error("HTTPS enabled should mount certs volume")
	}
	if strings.Contains(compose, "certificatesresolvers") {
		t.Error("no ACME email should NOT have ACME config")
	}
}

func TestTraefikTemplateHTTPSWithACME(t *testing.T) {
	r, _ := NewRegistry(templates.FS)
	renderer := NewTemplateRenderer(r)
	vals := traefikTestValues()
	vals["https_enabled"] = "true"
	vals["certs_dir"] = "/opt/homelabctl/data/traefik/certs"
	vals["acme_email"] = "user@example.com"

	files, err := renderer.RenderAllFiles("traefik", vals)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	compose := files["docker-compose.yml"]

	if !strings.Contains(compose, "providers.file.directory=/dynamic") {
		t.Error("should have file provider")
	}
	if !strings.Contains(compose, "certificatesresolvers.letsencrypt.acme.email=user@example.com") {
		t.Error("should have ACME email config")
	}
	if !strings.Contains(compose, "acme.httpchallenge.entrypoint=web") {
		t.Error("should have ACME HTTP challenge")
	}
}

func traefikTestValues() map[string]string {
	return map[string]string{
		"http_port": "80", "https_port": "443", "dashboard_port": "8081",
		"dashboard_enabled": "true", "https_enabled": "false", "acme_email": "",
		"certs_dir": "", "log_level": "INFO", "timezone": "UTC",
		"data_dir": "/opt/homelabctl/data/traefik", "network": "homelabctl-net",
	}
}

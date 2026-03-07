package app

import (
	"strings"
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestNextcloudTemplateHTTPSEnabled(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewTemplateRenderer(r)
	vals := nextcloudTestValues()
	vals["https_enabled"] = "true"
	vals["ca_cert_path"] = "/opt/homelabctl/data/traefik/certs/ca.crt"

	files, err := renderer.RenderAllFiles("nextcloud", vals)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	compose := files["docker-compose.yml"]

	if !strings.Contains(compose, "OVERWRITEPROTOCOL=https") {
		t.Error("HTTPS enabled should set OVERWRITEPROTOCOL=https")
	}
	if !strings.Contains(compose, "nextcloud.x1.local") {
		t.Error("trusted domains should include routing_domain")
	}
	if !strings.Contains(compose, "x1.local") {
		t.Error("trusted domains should include hostname.domain")
	}
	if !strings.Contains(compose, "ca-bundle.crt:/etc/ssl/certs/ca-certificates.crt:ro") {
		t.Error("HTTPS enabled should mount CA bundle for system trust")
	}
	if !strings.Contains(compose, "homelabctl-ca.crt:ro") {
		t.Error("HTTPS enabled should mount individual CA cert for Nextcloud import")
	}
	if !strings.Contains(compose, "host-gateway") {
		t.Error("should include extra_hosts for hairpin routing")
	}
}

func TestNextcloudTemplateHTTPSDisabled(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewTemplateRenderer(r)
	vals := nextcloudTestValues()

	files, err := renderer.RenderAllFiles("nextcloud", vals)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	compose := files["docker-compose.yml"]

	if strings.Contains(compose, "OVERWRITEPROTOCOL") {
		t.Error("HTTPS disabled should NOT set OVERWRITEPROTOCOL")
	}
	if !strings.Contains(compose, "nextcloud.x1.local") {
		t.Error("trusted domains should include routing_domain even without HTTPS")
	}
	if strings.Contains(compose, "ca-bundle.crt") {
		t.Error("HTTPS disabled should NOT mount CA bundle")
	}
}

func nextcloudTestValues() map[string]string {
	return map[string]string{
		"hostname": "x1", "domain": "local",
		"web_port": "8080", "network": "homelabctl-net",
		"data_dir": "/opt/homelabctl/data/nextcloud",
		"timezone": "UTC", "db_name": "nextcloud",
		"db_user": "nextcloud", "db_user_password": "testpass",
		"db_password": "testrootpass",
		"nextcloud_admin_user": "admin",
		"nextcloud_admin_password": "adminpass",
		"routing_domain": "nextcloud.x1.local",
		"routing_url": "http://nextcloud.x1.local",
		"https_enabled": "false",
		"ca_cert_path": "",
	}
}

package config

import (
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("hostname is set", func(t *testing.T) {
		// DefaultConfig reads os.Hostname, which should return something on any machine.
		if cfg.Hostname == "" {
			t.Error("DefaultConfig() hostname is empty")
		}
	})

	t.Run("apps_dir has default", func(t *testing.T) {
		if cfg.AppsDir != "/opt/homelabctl/apps" {
			t.Errorf("expected AppsDir=/opt/homelabctl/apps, got %q", cfg.AppsDir)
		}
	})

	t.Run("data_dir has default", func(t *testing.T) {
		if cfg.DataDir != "/opt/homelabctl/data" {
			t.Errorf("expected DataDir=/opt/homelabctl/data, got %q", cfg.DataDir)
		}
	})

	t.Run("web_port has default", func(t *testing.T) {
		if cfg.Network.WebPort != 8080 {
			t.Errorf("expected WebPort=8080, got %d", cfg.Network.WebPort)
		}
	})

	t.Run("compose_command has default", func(t *testing.T) {
		if cfg.Docker.ComposeCommand != "docker compose" {
			t.Errorf("expected ComposeCommand='docker compose', got %q", cfg.Docker.ComposeCommand)
		}
	})

	t.Run("backup retention defaults", func(t *testing.T) {
		if cfg.Backup.Retention.KeepDaily != 7 {
			t.Errorf("expected KeepDaily=7, got %d", cfg.Backup.Retention.KeepDaily)
		}
		if cfg.Backup.Retention.KeepWeekly != 4 {
			t.Errorf("expected KeepWeekly=4, got %d", cfg.Backup.Retention.KeepWeekly)
		}
		if cfg.Backup.Retention.KeepMonthly != 6 {
			t.Errorf("expected KeepMonthly=6, got %d", cfg.Backup.Retention.KeepMonthly)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Run("default config is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		errs := Validate(cfg)
		if len(errs) > 0 {
			t.Errorf("Validate(DefaultConfig()) returned errors: %v", errs)
		}
	})

	t.Run("empty hostname", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Hostname = ""
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "hostname") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for empty hostname")
		}
	})

	t.Run("bad port zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Network.WebPort = 0
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "web_port") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for port 0")
		}
	})

	t.Run("bad port too high", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Network.WebPort = 70000
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "web_port") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for port 70000")
		}
	})

	t.Run("empty apps_dir", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AppsDir = ""
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "apps_dir") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for empty apps_dir")
		}
	})

	t.Run("empty compose command", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Docker.ComposeCommand = ""
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "compose_command") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for empty compose_command")
		}
	})

	t.Run("backup enabled without repo", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Backup.Enabled = true
		cfg.Backup.BorgRepo = ""
		errs := Validate(cfg)
		found := false
		for _, e := range errs {
			if strings.Contains(e, "borg_repo") {
				found = true
			}
		}
		if !found {
			t.Error("expected validation error for empty borg_repo with backup enabled")
		}
	})

	t.Run("ingress https enabled without acme_email is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Ingress.Enabled = true
		cfg.Ingress.Provider = "traefik"
		cfg.Ingress.HTTPS.Enabled = true
		cfg.Ingress.HTTPS.AcmeEmail = ""
		errs := Validate(cfg)
		if len(errs) > 0 {
			t.Errorf("HTTPS without ACME email should be valid (local CA only), got: %v", errs)
		}
	})

	t.Run("ingress https enabled with acme_email is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Ingress.Enabled = true
		cfg.Ingress.Provider = "traefik"
		cfg.Ingress.HTTPS.Enabled = true
		cfg.Ingress.HTTPS.AcmeEmail = "user@example.com"
		errs := Validate(cfg)
		if len(errs) > 0 {
			t.Errorf("HTTPS with ACME email should be valid, got: %v", errs)
		}
	})
}

func TestAppDir(t *testing.T) {
	cfg := DefaultConfig()
	dir := cfg.AppDir("nextcloud")
	expected := "/opt/homelabctl/apps/nextcloud"
	if dir != expected {
		t.Errorf("AppDir(nextcloud) = %q, want %q", dir, expected)
	}
}

func TestDataPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.DataPath("nextcloud")
	expected := "/opt/homelabctl/data/nextcloud"
	if path != expected {
		t.Errorf("DataPath(nextcloud) = %q, want %q", path, expected)
	}
}

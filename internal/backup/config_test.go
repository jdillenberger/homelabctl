package backup

import (
	"strings"
	"testing"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

func TestGenerateConfig(t *testing.T) {
	backupCfg := config.BackupConfig{
		Enabled:  true,
		BorgRepo: "/mnt/backup/borg",
		Retention: config.RetentionConfig{
			KeepDaily:   7,
			KeepWeekly:  4,
			KeepMonthly: 6,
		},
	}

	t.Run("basic config without backup meta", func(t *testing.T) {
		meta := &app.AppMeta{
			Name: "testapp",
		}
		result := GenerateConfig("testapp", meta, backupCfg, "/opt/homelabctl/data")

		if !strings.Contains(result, "repositories:") {
			t.Error("expected repositories: section")
		}
		if !strings.Contains(result, "/mnt/backup/borg") {
			t.Error("expected borg repo path")
		}
		if !strings.Contains(result, "source_directories:") {
			t.Error("expected source_directories: section")
		}
		// Without backup paths, should use default data dir
		if !strings.Contains(result, "/opt/homelabctl/data/testapp") {
			t.Errorf("expected default data path, got:\n%s", result)
		}
		if !strings.Contains(result, "keep_daily: 7") {
			t.Error("expected keep_daily: 7")
		}
		if !strings.Contains(result, "keep_weekly: 4") {
			t.Error("expected keep_weekly: 4")
		}
		if !strings.Contains(result, "keep_monthly: 6") {
			t.Error("expected keep_monthly: 6")
		}
	})

	t.Run("config with custom backup paths", func(t *testing.T) {
		meta := &app.AppMeta{
			Name: "nextcloud",
			Backup: &app.BackupMeta{
				Paths: []string{"html", "db"},
			},
		}
		result := GenerateConfig("nextcloud", meta, backupCfg, "/opt/homelabctl/data")

		if !strings.Contains(result, "- html") {
			t.Error("expected custom path 'html'")
		}
		if !strings.Contains(result, "- db") {
			t.Error("expected custom path 'db'")
		}
		// Should NOT contain the default data path
		if strings.Contains(result, "/opt/homelabctl/data/nextcloud") {
			t.Error("should not contain default data path when custom paths are set")
		}
	})

	t.Run("config with hooks", func(t *testing.T) {
		meta := &app.AppMeta{
			Name: "nextcloud",
			Backup: &app.BackupMeta{
				Paths:    []string{"html"},
				PreHook:  "docker exec db dump",
				PostHook: "rm /tmp/dump.sql",
			},
		}
		result := GenerateConfig("nextcloud", meta, backupCfg, "/opt/homelabctl/data")

		if !strings.Contains(result, "before_actions:") {
			t.Error("expected before_actions: section for pre hook")
		}
		if !strings.Contains(result, "docker exec db dump") {
			t.Error("expected pre hook command")
		}
		if !strings.Contains(result, "after_actions:") {
			t.Error("expected after_actions: section for post hook")
		}
		if !strings.Contains(result, "rm /tmp/dump.sql") {
			t.Error("expected post hook command")
		}
	})

	t.Run("config without hooks has no actions sections", func(t *testing.T) {
		meta := &app.AppMeta{
			Name: "simple",
			Backup: &app.BackupMeta{
				Paths: []string{"data"},
			},
		}
		result := GenerateConfig("simple", meta, backupCfg, "/opt/homelabctl/data")

		if strings.Contains(result, "before_actions:") {
			t.Error("should not contain before_actions when no pre hook")
		}
		if strings.Contains(result, "after_actions:") {
			t.Error("should not contain after_actions when no post hook")
		}
	})
}

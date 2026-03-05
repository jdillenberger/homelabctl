package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// GenerateConfig creates a borgmatic YAML config string for the given app.
func GenerateConfig(appName string, meta *app.AppMeta, backupCfg config.BackupConfig, dataDir string) string {
	var b strings.Builder

	b.WriteString("repositories:\n")
	b.WriteString(fmt.Sprintf("  - path: %s\n", backupCfg.BorgRepo))
	b.WriteString("    label: homelabctl\n")
	b.WriteString("\n")

	b.WriteString("source_directories:\n")
	if meta.Backup != nil && len(meta.Backup.Paths) > 0 {
		for _, p := range meta.Backup.Paths {
			b.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	} else {
		b.WriteString(fmt.Sprintf("  - %s\n", filepath.Join(dataDir, appName)))
	}
	b.WriteString("\n")

	b.WriteString("retention:\n")
	b.WriteString(fmt.Sprintf("  keep_daily: %d\n", backupCfg.Retention.KeepDaily))
	b.WriteString(fmt.Sprintf("  keep_weekly: %d\n", backupCfg.Retention.KeepWeekly))
	b.WriteString(fmt.Sprintf("  keep_monthly: %d\n", backupCfg.Retention.KeepMonthly))

	if meta.Backup != nil {
		if meta.Backup.PreHook != "" {
			b.WriteString("\nbefore_actions:\n")
			b.WriteString(fmt.Sprintf("  - \"%s\"\n", meta.Backup.PreHook))
		}
		if meta.Backup.PostHook != "" {
			b.WriteString("\nafter_actions:\n")
			b.WriteString(fmt.Sprintf("  - \"%s\"\n", meta.Backup.PostHook))
		}
	}

	return b.String()
}

// WriteConfig writes borgmatic config content to a file in the given directory.
func WriteConfig(appName, content, configDir string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", configDir, err)
	}
	path := filepath.Join(configDir, "borgmatic.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing borgmatic config for %s: %w", appName, err)
	}
	return nil
}

// ConfigPath returns the expected borgmatic config file path for an app.
func ConfigPath(appsDir, appName string) string {
	return filepath.Join(appsDir, appName, "borgmatic.yaml")
}

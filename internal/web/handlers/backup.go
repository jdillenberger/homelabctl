package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// BackupPageData holds data for the backup template.
type BackupPageData struct {
	Enabled   bool
	BorgRepo  string
	Schedule  string
	Retention config.RetentionConfig
	Apps      []*app.DeployedApp
}

// HandleBackupPage renders the backup overview page.
func (h *Handler) HandleBackupPage(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()
	var apps []*app.DeployedApp
	for _, name := range deployed {
		info, err := h.manager.GetDeployedInfo(name)
		if err != nil {
			continue
		}
		apps = append(apps, info)
	}

	data := BackupPageData{
		Enabled:   h.cfg.Backup.Enabled,
		BorgRepo:  h.cfg.Backup.BorgRepo,
		Schedule:  h.cfg.Backup.Schedule,
		Retention: h.cfg.Backup.Retention,
		Apps:      apps,
	}

	return c.Render(http.StatusOK, "backup.html", data)
}

// HandleBackupCreate triggers a manual backup and returns an htmx snippet.
func (h *Handler) HandleBackupCreate(c echo.Context) error {
	if !h.cfg.Backup.Enabled {
		return c.HTML(http.StatusOK, `<p style="color:red;">Backups are disabled in configuration.</p>`)
	}

	deployed, err := h.manager.ListDeployed()
	if err != nil {
		return c.HTML(http.StatusOK, fmt.Sprintf(
			`<p style="color:red;">Failed to list deployed apps: %s</p>`, err))
	}

	borg := backup.NewBorg(h.runner)
	registry := h.manager.Registry()
	var succeeded, failed []string

	for _, appName := range deployed {
		configFile := backup.ConfigPath(h.cfg.AppsDir, appName)
		if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
			continue
		}

		meta, _ := registry.Get(appName)

		// Run pre-hook if defined.
		if meta != nil && meta.Backup != nil && meta.Backup.PreHook != "" {
			if _, hookErr := h.runner.Run("sh", "-c", meta.Backup.PreHook); hookErr != nil {
				slog.Error("Backup pre-hook failed", "app", appName, "error", hookErr)
				failed = append(failed, appName)
				continue
			}
		}

		if _, borgErr := borg.Create(configFile); borgErr != nil {
			slog.Error("Backup failed", "app", appName, "error", borgErr)
			failed = append(failed, appName)
			continue
		}

		// Run post-hook if defined.
		if meta != nil && meta.Backup != nil && meta.Backup.PostHook != "" {
			if _, hookErr := h.runner.Run("sh", "-c", meta.Backup.PostHook); hookErr != nil {
				slog.Error("Backup post-hook failed", "app", appName, "error", hookErr)
			}
		}

		succeeded = append(succeeded, appName)
	}

	var msg strings.Builder
	if len(succeeded) > 0 {
		msg.WriteString(fmt.Sprintf(`<p style="color:green;">Backup succeeded for: %s</p>`, strings.Join(succeeded, ", ")))
	}
	if len(failed) > 0 {
		msg.WriteString(fmt.Sprintf(`<p style="color:red;">Backup failed for: %s</p>`, strings.Join(failed, ", ")))
	}
	if len(succeeded) == 0 && len(failed) == 0 {
		msg.WriteString(`<p>No apps with backup configuration found.</p>`)
	}
	return c.HTML(http.StatusOK, msg.String())
}

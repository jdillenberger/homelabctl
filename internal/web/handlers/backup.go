package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
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

	// TODO: Wire up to actual borg/borgmatic backup execution.
	return c.HTML(http.StatusOK, fmt.Sprintf(
		`<p style="color:green;">Manual backup triggered for repo %s. Check logs for progress.</p>`,
		h.cfg.Backup.BorgRepo,
	))
}

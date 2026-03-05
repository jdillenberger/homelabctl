package handlers

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/health"
)

// APIHealth returns a JSON health check response.
func (h *Handler) APIHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"hostname": h.cfg.Hostname,
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

// APIStats returns JSON system stats.
func (h *Handler) APIStats(c echo.Context) error {
	return c.JSON(http.StatusOK, statsJSON())
}

// APIHealthHistory returns health check history as JSON.
func (h *Handler) APIHealthHistory(c echo.Context) error {
	monitor := health.NewMonitor(h.cfg.DataDir, h.cfg.Health.MaxHistory)
	records, err := monitor.LoadHistory()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if records == nil {
		records = []health.CheckRecord{}
	}
	return c.JSON(http.StatusOK, records)
}

// APIApps returns a JSON list of deployed apps.
func (h *Handler) APIApps(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()

	type appInfo struct {
		Name       string `json:"name"`
		Template   string `json:"template"`
		Version    string `json:"version"`
		DeployedAt string `json:"deployed_at"`
	}

	var apps []appInfo
	for _, name := range deployed {
		info, err := h.manager.GetDeployedInfo(name)
		if err != nil {
			continue
		}
		apps = append(apps, appInfo{
			Name:       info.Name,
			Template:   info.Template,
			Version:    info.Version,
			DeployedAt: info.DeployedAt.Format(time.RFC3339),
		})
	}

	if apps == nil {
		apps = []appInfo{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"apps":  apps,
		"count": len(apps),
	})
}

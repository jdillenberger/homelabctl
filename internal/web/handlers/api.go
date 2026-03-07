package handlers

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
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

// APIAppsHealth returns cached health status for all deployed apps as JSON.
func (h *Handler) APIAppsHealth(c echo.Context) error {
	all := h.healthCache.All()
	// Filter out apps without a healthcheck.
	var results []app.CachedHealthResult
	for _, r := range all {
		if r.Status != app.HealthStatusNone {
			results = append(results, r)
		}
	}
	if results == nil {
		results = []app.CachedHealthResult{}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"results": results,
		"count":   len(results),
	})
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

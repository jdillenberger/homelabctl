package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
)

// DashboardData holds data for the dashboard template.
type DashboardData struct {
	Hostname     string
	Stats        SystemStats
	DeployedApps []*app.DeployedApp
}

// Dashboard renders the main dashboard page.
func (h *Handler) Dashboard(c echo.Context) error {
	stats := collectStats()

	deployed, _ := h.manager.ListDeployed()
	var apps []*app.DeployedApp
	for _, name := range deployed {
		info, err := h.manager.GetDeployedInfo(name)
		if err != nil {
			continue
		}
		apps = append(apps, info)
	}

	data := DashboardData{
		Hostname:     h.cfg.Hostname,
		Stats:        stats,
		DeployedApps: apps,
	}

	return c.Render(http.StatusOK, "dashboard.html", data)
}

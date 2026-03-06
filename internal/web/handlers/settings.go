package handlers

import (
	"net/http"
	"runtime"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/config"
)

// SettingsPageData holds data for the settings template.
type SettingsPageData struct {
	BasePage
	Version string
	Uptime  string
	Config  *config.Config
}

// HandleSettingsPage renders the settings page.
func (h *Handler) HandleSettingsPage(c echo.Context) error {
	stats := collectStats()

	data := SettingsPageData{
		BasePage: h.basePage(),
		Version:  runtime.Version(),
		Uptime:   stats.Uptime,
		Config:   h.cfg,
	}

	return c.Render(http.StatusOK, "settings.html", data)
}

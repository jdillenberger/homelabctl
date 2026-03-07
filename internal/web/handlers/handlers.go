package handlers

import (
	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
)

// BasePage holds common template data shared across all pages.
type BasePage struct {
	Hostname string
	Domain   string
	NavColor string
}

// Handler holds shared dependencies for all route handlers.
type Handler struct {
	cfg     *config.Config
	manager *app.Manager
	runner  *exec.Runner
	compose *app.Compose
}

// New creates a new Handler with all dependencies.
func New(cfg *config.Config, manager *app.Manager, runner *exec.Runner) *Handler {
	return &Handler{
		cfg:     cfg,
		manager: manager,
		runner:  runner,
		compose: app.NewCompose(runner, cfg.Docker.ComposeCommand),
	}
}

func (h *Handler) basePage() BasePage {
	return BasePage{
		Hostname: h.cfg.Hostname,
		Domain:   h.cfg.Network.Domain,
		NavColor: h.cfg.Web.NavColor,
	}
}

// Register registers all routes on the Echo instance.
func (h *Handler) Register(e *echo.Echo) {
	// Dashboard
	e.GET("/", h.Dashboard)
	e.GET("/dashboard/health", h.DashboardHealth)
	e.GET("/dashboard/peers", h.DashboardPeers)

	// Stats
	e.GET("/stats/partial", h.StatsPartial)
	e.GET("/stats/compact", h.StatsCompact)
	e.GET("/stats/dashboard", h.StatsDashboard)

	// Apps
	e.GET("/apps", h.AppsList)
	e.GET("/apps/:name", h.AppDetail)
	e.POST("/apps/:name/start", h.AppStart)
	e.POST("/apps/:name/stop", h.AppStop)
	e.POST("/apps/:name/restart", h.AppRestart)
	e.GET("/apps/:name/logs", h.AppLogs)
	e.GET("/apps/:name/logs/stream", h.AppLogsStream)

	// Fleet (discovery-only)
	e.GET("/fleet", h.HandleFleetPage)
	e.GET("/api/fleet", h.HandleFleetAPI)

	// Backups
	e.GET("/backups", h.HandleBackupPage)
	e.POST("/backups/create", h.HandleBackupCreate)

	// Alerts
	e.GET("/alerts", h.HandleAlertsPage)
	e.GET("/alerts/partial", h.AlertsPartial)
	e.GET("/api/alerts/rules", h.APIAlertRules)
	e.GET("/api/alerts/history", h.APIAlertHistory)

	// Settings
	e.GET("/settings", h.HandleSettingsPage)

	// API
	e.GET("/api/health", h.APIHealth)
	e.GET("/api/stats", h.APIStats)
	e.GET("/api/apps", h.APIApps)

}

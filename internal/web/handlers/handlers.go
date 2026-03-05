package handlers

import (
	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
)

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

// Register registers all routes on the Echo instance.
func (h *Handler) Register(e *echo.Echo) {
	// Dashboard
	e.GET("/", h.Dashboard)

	// Stats
	e.GET("/stats/partial", h.StatsPartial)

	// Apps
	e.GET("/apps", h.AppsList)
	e.GET("/apps/:name", h.AppDetail)
	e.POST("/apps/:name/start", h.AppStart)
	e.POST("/apps/:name/stop", h.AppStop)
	e.POST("/apps/:name/restart", h.AppRestart)
	e.GET("/apps/:name/logs", h.AppLogs)

	// Fleet
	e.GET("/fleet", h.HandleFleetPage)
	e.GET("/api/fleet", h.HandleFleetAPI)
	e.POST("/api/fleet/deploy", h.HandleFleetDeploy)

	// Backups
	e.GET("/backups", h.HandleBackupPage)
	e.POST("/backups/create", h.HandleBackupCreate)

	// Settings
	e.GET("/settings", h.HandleSettingsPage)

	// API
	e.GET("/api/health", h.APIHealth)
	e.GET("/api/stats", h.APIStats)
	e.GET("/api/apps", h.APIApps)
}

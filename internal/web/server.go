package web

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/web/handlers"
	"github.com/jdillenberger/homelabctl/internal/web/templates"
	"github.com/jdillenberger/homelabctl/static"
)

// Server holds the Echo instance and dependencies.
type Server struct {
	Echo        *echo.Echo
	cfg         *config.Config
	manager     *app.Manager
	healthCache *app.HealthCache
}

// NewServer creates and configures a new web server.
// When devMode is true, a livereload script is injected into pages.
func NewServer(cfg *config.Config, manager *app.Manager, runner *exec.Runner, devMode bool) (*Server, error) {
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogURI:    true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			c.Logger().Infof("%s %s %d", v.Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	// Livereload injection in dev mode
	if devMode {
		e.Use(livereloadMiddleware())
	}

	// Template renderer
	renderer, err := templates.NewRenderer()
	if err != nil {
		return nil, err
	}
	e.Renderer = renderer

	// Static files from embedded FS
	staticFS, err := fs.Sub(static.FS, ".")
	if err != nil {
		return nil, err
	}
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))))

	// Create and start health cache (polls docker healthcheck status in background)
	compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
	healthCache := app.NewHealthCache(
		compose,
		cfg.AppsDir,
		manager.ListDeployed,
		30*time.Second,
		2*time.Minute,
	)
	healthCache.Start()

	s := &Server{
		Echo:        e,
		cfg:         cfg,
		manager:     manager,
		healthCache: healthCache,
	}

	// Register handlers
	h := handlers.New(cfg, manager, runner, healthCache)
	h.Register(e)

	return s, nil
}

// Start starts the HTTP server on the given address.
func (s *Server) Start(addr string) error {
	return s.Echo.Start(addr)
}

// Shutdown gracefully stops background tasks and the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.healthCache.Stop()
	return s.Echo.Shutdown(ctx)
}

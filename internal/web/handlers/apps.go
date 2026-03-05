package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
)

// AppsListData holds data for the apps list template.
type AppsListData struct {
	DeployedApps []*app.DeployedApp
	Templates    []*app.AppMeta
	DeployedSet  map[string]bool
}

// AppDetailData holds data for the app detail template.
type AppDetailData struct {
	App    *app.DeployedApp
	Status string
}

// AppsList renders the apps list page.
func (h *Handler) AppsList(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()
	deployedSet := make(map[string]bool)
	var apps []*app.DeployedApp
	for _, name := range deployed {
		deployedSet[name] = true
		info, err := h.manager.GetDeployedInfo(name)
		if err != nil {
			continue
		}
		apps = append(apps, info)
	}

	data := AppsListData{
		DeployedApps: apps,
		Templates:    h.manager.Registry().All(),
		DeployedSet:  deployedSet,
	}

	return c.Render(http.StatusOK, "apps.html", data)
}

// AppDetail renders the app detail page.
func (h *Handler) AppDetail(c echo.Context) error {
	name := c.Param("name")

	info, err := h.manager.GetDeployedInfo(name)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("app %s not found", name))
	}

	status, _ := h.manager.Status(name)

	data := AppDetailData{
		App:    info,
		Status: status,
	}

	return c.Render(http.StatusOK, "app_detail.html", data)
}

// AppStart starts an app and returns an htmx response.
func (h *Handler) AppStart(c echo.Context) error {
	name := c.Param("name")
	if err := h.manager.Start(name); err != nil {
		return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:red;">Error starting %s: %s</p>`, name, err.Error()))
	}
	return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:green;">%s started successfully.</p>`, name))
}

// AppStop stops an app and returns an htmx response.
func (h *Handler) AppStop(c echo.Context) error {
	name := c.Param("name")
	if err := h.manager.Stop(name); err != nil {
		return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:red;">Error stopping %s: %s</p>`, name, err.Error()))
	}
	return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:green;">%s stopped successfully.</p>`, name))
}

// AppRestart restarts an app and returns an htmx response.
func (h *Handler) AppRestart(c echo.Context) error {
	name := c.Param("name")
	if err := h.manager.Restart(name); err != nil {
		return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:red;">Error restarting %s: %s</p>`, name, err.Error()))
	}
	return c.HTML(http.StatusOK, fmt.Sprintf(`<p style="color:green;">%s restarted successfully.</p>`, name))
}

// AppLogs streams app logs via SSE.
func (h *Handler) AppLogs(c echo.Context) error {
	name := c.Param("name")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	appDir := h.cfg.AppDir(name)

	// Use a flushing writer wrapper
	fw := &flushWriter{w: c.Response()}
	err := h.compose.Logs(appDir, fw, true, 100)
	if err != nil {
		return err
	}

	return nil
}

type flushWriter struct {
	w *echo.Response
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.w.Flush()
	return n, err
}

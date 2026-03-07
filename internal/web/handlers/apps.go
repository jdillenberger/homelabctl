package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
)

// AppsListData holds data for the apps list template.
type AppsListData struct {
	BasePage
	DeployedApps []*app.DeployedApp
	Templates    []*app.AppMeta
	DeployedSet  map[string]bool
}

// ContainerStatus represents a parsed docker compose ps row.
type ContainerStatus struct {
	Service string
	State   string
	Status  string
	Ports   string
	Running bool
}

// AppDetailData holds data for the app detail template.
type AppDetailData struct {
	BasePage
	App        *app.DeployedApp
	Containers []ContainerStatus
	StatusRaw  string
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
		BasePage:     h.basePage(),
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

	data := AppDetailData{
		BasePage: h.basePage(),
		App:      info,
	}

	// Try JSON format first for structured display
	appDir := h.cfg.AppDir(name)
	result, err := h.compose.PSJson(appDir)
	if err == nil && result.Stdout != "" {
		data.Containers = parseComposeJSON(result.Stdout)
	}

	// Fallback to raw table output
	if len(data.Containers) == 0 {
		status, _ := h.manager.Status(name)
		data.StatusRaw = status
	}

	return c.Render(http.StatusOK, "app_detail.html", data)
}

// parseComposeJSON parses docker compose ps --format json output.
// Docker compose outputs one JSON object per line (NDJSON).
func parseComposeJSON(raw string) []ContainerStatus {
	var containers []ContainerStatus

	// Docker compose ps --format json outputs one JSON object per line
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var obj struct {
			Service string `json:"Service"`
			State   string `json:"State"`
			Status  string `json:"Status"`
			Ports   string `json:"Ports"`
			Name    string `json:"Name"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}

		cs := ContainerStatus{
			Service: obj.Service,
			State:   obj.State,
			Status:  obj.Status,
			Ports:   obj.Ports,
			Running: obj.State == "running",
		}
		if cs.Service == "" {
			cs.Service = obj.Name
		}
		containers = append(containers, cs)
	}

	return containers
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

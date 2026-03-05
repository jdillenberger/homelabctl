package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// FleetPageData holds data for the fleet template.
type FleetPageData struct {
	FleetName    string
	DomainSuffix string
	Hosts        []config.FleetHost
}

// HandleFleetPage serves the fleet overview HTML page using the template renderer.
func (h *Handler) HandleFleetPage(c echo.Context) error {
	fleetCfg, err := config.LoadFleetConfig()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("loading fleet config: %v", err))
	}

	data := FleetPageData{
		FleetName:    fleetCfg.Fleet.Name,
		DomainSuffix: fleetCfg.Defaults.DomainSuffix,
		Hosts:        fleetCfg.Hosts,
	}

	return c.Render(http.StatusOK, "fleet.html", data)
}

// HandleFleetAPI returns fleet status as JSON.
func (h *Handler) HandleFleetAPI(c echo.Context) error {
	fleetCfg, err := config.LoadFleetConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("loading fleet config: %v", err),
		})
	}

	return c.JSON(http.StatusOK, fleetCfg)
}

// HandleFleetDeploy accepts a deploy request from a peer.
func (h *Handler) HandleFleetDeploy(c echo.Context) error {
	fleetCfg, err := config.LoadFleetConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("loading fleet config: %v", err),
		})
	}

	// Validate PSK
	if fleetCfg.Fleet.Secret != "" {
		secret := c.Request().Header.Get("X-Fleet-Secret")
		if secret != fleetCfg.Fleet.Secret {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "unauthorized: invalid fleet secret",
			})
		}
	}

	var req struct {
		App    string            `json:"app"`
		Values map[string]string `json:"values,omitempty"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}

	req.App = strings.TrimSpace(req.App)
	if req.App == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "app name is required",
		})
	}

	if err := h.manager.Deploy(req.App, app.DeployOptions{
		Values:  req.Values,
		DryRun:  false,
		Confirm: true, // API calls skip confirmation
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("deploy failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "deployed",
		"app":    req.App,
	})
}

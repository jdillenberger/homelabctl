package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

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


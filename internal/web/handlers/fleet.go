package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/peerscan"
)

// FleetPageData holds data for the fleet template.
type FleetPageData struct {
	BasePage
	FleetName string
	Self      peerscan.Peer
	Peers     []peerscan.Peer
}

// HandleFleetPage serves the fleet overview HTML page using the template renderer.
func (h *Handler) HandleFleetPage(c echo.Context) error {
	resp, err := h.peerClient.Peers()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("querying peer-scanner: %v", err))
	}

	data := FleetPageData{
		BasePage:  h.basePage(),
		FleetName: resp.Fleet.Name,
		Self:      resp.Self,
		Peers:     resp.Peers,
	}

	return c.Render(http.StatusOK, "fleet.html", data)
}

// HandleFleetAPI returns fleet status as JSON from the peer-scanner daemon.
func (h *Handler) HandleFleetAPI(c echo.Context) error {
	resp, err := h.peerClient.Peers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("querying peer-scanner: %v", err),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

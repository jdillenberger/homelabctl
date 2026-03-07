package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/mdns"
)

// APIRoutingStatus returns the list of domains currently routed by traefik.
// Used by the dashboard JS to upgrade app URLs from host:port to routing domains.
func (h *Handler) APIRoutingStatus(c echo.Context) error {
	var domains []string
	if h.cfg.Routing.Enabled {
		active, err := mdns.DiscoverTraefikDomains(h.runner, h.cfg.Docker.Runtime)
		if err == nil {
			for domain := range active {
				domains = append(domains, domain)
			}
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"domains": domains})
}

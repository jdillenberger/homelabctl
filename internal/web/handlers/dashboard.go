package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/mdns"
)

// PortalApp represents an app displayed on the portal dashboard.
type PortalApp struct {
	Name        string
	Description string
	Version     string
	DeployedAt  string
	Health      string // "healthy", "unhealthy", "unknown"
	AccessURL   string
}

// FleetPeer represents a discovered peer server.
type FleetPeer struct {
	Hostname string
	Address  string
	Port     int
	DashURL  string
	Apps     []string
}

// DashboardData holds data for the dashboard template.
type DashboardData struct {
	Hostname  string
	Domain    string
	Stats     SystemStats
	Apps      []PortalApp
	Peers     []FleetPeer
	ShowStats bool
}

// Dashboard renders the main portal dashboard page.
func (h *Handler) Dashboard(c echo.Context) error {
	stats := collectStats()

	deployed, _ := h.manager.ListDeployed()
	registry := h.manager.Registry()
	checker := app.NewHealthChecker()
	compose := app.NewCompose(h.runner, h.cfg.Docker.ComposeCommand)

	var portalApps []PortalApp
	for _, name := range deployed {
		info, err := h.manager.GetDeployedInfo(name)
		if err != nil {
			continue
		}

		pa := PortalApp{
			Name:       info.Name,
			Version:    info.Version,
			DeployedAt: info.DeployedAt.Format("2006-01-02"),
			Health:     "unknown",
		}

		// Get description from registry
		if meta, ok := registry.Get(name); ok {
			pa.Description = meta.Description

			// Run health check
			result := checker.CheckApp(meta, compose, h.cfg.AppDir(name))
			pa.Health = string(result.Status)

			// Build access URL from port mappings
			hostname := h.cfg.Hostname
			domain := h.cfg.Network.Domain
			fqdn := hostname + "." + domain
			for _, p := range meta.Ports {
				if p.ValueName != "" {
					if portVal, ok := info.Values[p.ValueName]; ok {
						scheme := "http"
						if strings.Contains(strings.ToLower(p.Description), "https") {
							scheme = "https"
						}
						pa.AccessURL = fmt.Sprintf("%s://%s:%s", scheme, fqdn, portVal)
						break
					}
				} else if p.Host > 0 {
					pa.AccessURL = fmt.Sprintf("http://%s:%d", fqdn, p.Host)
					break
				}
			}
		}

		portalApps = append(portalApps, pa)
	}

	// Discover fleet peers (non-blocking, short timeout)
	var peers []FleetPeer
	if h.cfg.MDNS.Enabled {
		hosts, err := mdns.Discover(2 * time.Second)
		if err == nil {
			for _, host := range hosts {
				peers = append(peers, FleetPeer{
					Hostname: host.Hostname,
					Address:  host.Address,
					Port:     host.Port,
					DashURL:  fmt.Sprintf("http://%s:%d", host.Address, host.Port),
					Apps:     host.Apps,
				})
			}
		}
	}

	// Also include fleet config hosts that weren't discovered
	fleetCfg, err := config.LoadFleetConfig()
	if err == nil {
		for _, host := range fleetCfg.Hosts {
			// Skip self
			if host.Hostname == h.cfg.Hostname {
				continue
			}
			// Skip if already discovered
			found := false
			for _, p := range peers {
				if p.Hostname == host.Hostname {
					found = true
					break
				}
			}
			if !found && host.Address != "" {
				port := host.Port
				if port == 0 {
					port = 8080
				}
				peers = append(peers, FleetPeer{
					Hostname: host.Hostname,
					Address:  host.Address,
					Port:     port,
					DashURL:  fmt.Sprintf("http://%s:%d", host.Address, port),
					Apps:     host.Apps,
				})
			}
		}
	}

	data := DashboardData{
		Hostname:  h.cfg.Hostname,
		Domain:    h.cfg.Network.Domain,
		Stats:     stats,
		Apps:      portalApps,
		Peers:     peers,
		ShowStats: true,
	}

	return c.Render(http.StatusOK, "dashboard.html", data)
}

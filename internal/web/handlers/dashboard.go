package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/alert"
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
	RoutingURL  string
	DisplayURL  string // protocol-stripped URL for display (prefers RoutingURL)
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
	BasePage
	Apps             []PortalApp
	ActiveAlertCount int
}

// Dashboard renders the main portal dashboard page.
// Stats, health checks, and peer discovery are loaded asynchronously via HTMX.
func (h *Handler) Dashboard(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()
	registry := h.manager.Registry()

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

		// Get description and URLs from registry (fast, no I/O)
		if meta, ok := registry.Get(name); ok {
			pa.Description = meta.Description

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

		// Set routing URL if available
		if info.Routing != nil && info.Routing.Enabled && len(info.Routing.Domains) > 0 {
			scheme := "http"
			if h.cfg.Routing.HTTPS.Enabled {
				scheme = "https"
			}
			pa.RoutingURL = fmt.Sprintf("%s://%s", scheme, info.Routing.Domains[0])
		}

		// Set display URL
		if pa.RoutingURL != "" {
			pa.DisplayURL = strings.TrimPrefix(strings.TrimPrefix(pa.RoutingURL, "https://"), "http://")
		} else if pa.AccessURL != "" {
			pa.DisplayURL = strings.TrimPrefix(strings.TrimPrefix(pa.AccessURL, "https://"), "http://")
		}

		portalApps = append(portalApps, pa)
	}

	// Count recent alerts (last 24h) — fast file read, no blocking I/O
	var alertCount int
	alertStore := alert.NewStore(h.cfg.DataDir)
	alertHistory, err := alertStore.LoadHistory()
	if err == nil {
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, a := range alertHistory {
			if a.Timestamp.After(cutoff) {
				alertCount++
			}
		}
	}

	data := DashboardData{
		BasePage:         h.basePage(),
		Apps:             portalApps,
		ActiveAlertCount: alertCount,
	}

	return c.Render(http.StatusOK, "dashboard.html", data)
}

// DashboardHealth returns out-of-band health badge updates for all deployed apps.
// Health checks run in parallel to minimize total wait time.
func (h *Handler) DashboardHealth(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()
	registry := h.manager.Registry()
	checker := app.NewHealthChecker()
	compose := app.NewCompose(h.runner, h.cfg.Docker.ComposeCommand)

	type healthResult struct {
		name   string
		health string
	}

	results := make([]healthResult, len(deployed))
	var wg sync.WaitGroup

	for i, name := range deployed {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			health := "unknown"
			if meta, ok := registry.Get(name); ok {
				r := checker.CheckApp(meta, compose, h.cfg.AppDir(name))
				health = string(r.Status)
			}
			results[i] = healthResult{name: name, health: health}
		}(i, name)
	}
	wg.Wait()

	var buf strings.Builder
	// Empty primary swap target content
	buf.WriteString("<span></span>")
	// OOB swaps for each health badge
	for _, r := range results {
		badgeClass := "badge-available"
		label := "unknown"
		switch r.health {
		case "healthy":
			badgeClass = "badge-running"
			label = "healthy"
		case "unhealthy":
			badgeClass = "badge-stopped"
			label = "down"
		}
		fmt.Fprintf(&buf, `<span id="health-%s" hx-swap-oob="true" class="badge %s">%s</span>`,
			html.EscapeString(r.name), badgeClass, label)
	}

	return c.HTML(http.StatusOK, buf.String())
}

// DashboardPeers returns the fleet peers section HTML, loaded asynchronously.
func (h *Handler) DashboardPeers(c echo.Context) error {
	var peers []FleetPeer

	if h.cfg.MDNS.Enabled {
		hosts, err := mdns.Discover(2 * time.Second)
		if err == nil {
			for _, host := range hosts {
				if host.Hostname == h.cfg.Hostname {
					continue
				}
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
			if host.Hostname == h.cfg.Hostname {
				continue
			}
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

	if len(peers) == 0 {
		return c.HTML(http.StatusOK, "")
	}

	var buf strings.Builder
	buf.WriteString(`<h3 class="section-title">Other Servers</h3>`)
	buf.WriteString(`<div class="peers-compact">`)
	for _, p := range peers {
		fmt.Fprintf(&buf, `<a href="%s" target="_blank" rel="noopener" class="peer-chip"><span class="peer-dot"></span>%s`,
			html.EscapeString(p.DashURL), html.EscapeString(p.Hostname))
		if len(p.Apps) > 0 {
			fmt.Fprintf(&buf, `<small>(%d)</small>`, len(p.Apps))
		}
		buf.WriteString(`</a>`)
	}
	buf.WriteString(`<a href="/fleet" style="font-size:0.8rem;">Fleet details &rarr;</a>`)
	buf.WriteString(`</div>`)

	return c.HTML(http.StatusOK, buf.String())
}

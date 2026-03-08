package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/alertclient"
	"github.com/jdillenberger/homelabctl/internal/app"
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

	// Use the request host so app links match how the user reached the dashboard.
	requestHost := c.Request().Host
	if idx := strings.LastIndex(requestHost, ":"); idx != -1 {
		requestHost = requestHost[:idx]
	}

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

		if meta, ok := registry.Get(name); ok {
			pa.Description = meta.Description

			for _, p := range meta.Ports {
				if p.ValueName != "" {
					if portVal, ok := info.Values[p.ValueName]; ok {
						scheme := "http"
						if strings.Contains(strings.ToLower(p.Description), "https") {
							scheme = "https"
						}
						pa.AccessURL = fmt.Sprintf("%s://%s:%s", scheme, requestHost, portVal)
						break
					}
				} else if p.Host > 0 {
					pa.AccessURL = fmt.Sprintf("http://%s:%d", requestHost, p.Host)
					break
				}
			}
		}

		// Set routing URL for JS upgrade (not used as initial href)
		if info.Routing != nil && info.Routing.Enabled && len(info.Routing.Domains) > 0 {
			scheme := "http"
			if h.cfg.Routing.HTTPS.Enabled {
				scheme = "https"
			}
			pa.RoutingURL = fmt.Sprintf("%s://%s", scheme, info.Routing.Domains[0])
		}

		// Display URL always derives from AccessURL (JS upgrades it if routing is active)
		if pa.AccessURL != "" {
			pa.DisplayURL = strings.TrimPrefix(strings.TrimPrefix(pa.AccessURL, "https://"), "http://")
		}

		portalApps = append(portalApps, pa)
	}

	// Count recent alerts (last 24h) from labalert
	var alertCount int
	if h.cfg.Labalert.URL != "" {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		client := alertclient.New(h.cfg.Labalert.URL)
		historyRaw, err := client.History(ctx, 500)
		if err == nil {
			var history []struct {
				Timestamp time.Time `json:"timestamp"`
			}
			if json.Unmarshal(historyRaw, &history) == nil {
				cutoff := time.Now().Add(-24 * time.Hour)
				for _, a := range history {
					if a.Timestamp.After(cutoff) {
						alertCount++
					}
				}
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
// Reads from the in-memory health cache (populated by background polling).
func (h *Handler) DashboardHealth(c echo.Context) error {
	deployed, _ := h.manager.ListDeployed()

	var buf strings.Builder
	buf.WriteString("<span></span>")

	for _, name := range deployed {
		r := h.healthCache.Get(name)

		// No badge for apps without a Docker healthcheck.
		if r.Status == app.HealthStatusNone {
			fmt.Fprintf(&buf, `<span id="health-%s" hx-swap-oob="true"></span>`,
				html.EscapeString(name))
			continue
		}

		badgeClass := "badge-available"
		label := "unknown"
		switch r.Status {
		case app.HealthStatusHealthy:
			badgeClass = "badge-running"
			label = "healthy"
		case app.HealthStatusUnhealthy:
			badgeClass = "badge-stopped"
			label = "down"
		case app.HealthStatusStarting:
			badgeClass = "badge-available"
			label = "starting"
		}
		fmt.Fprintf(&buf, `<span id="health-%s" hx-swap-oob="true" class="badge %s">%s</span>`,
			html.EscapeString(name), badgeClass, label)
	}

	return c.HTML(http.StatusOK, buf.String())
}

// DashboardPeers returns the fleet peers section HTML, loaded asynchronously.
func (h *Handler) DashboardPeers(c echo.Context) error {
	resp, err := h.peerClient.Peers()
	if err != nil || len(resp.Peers) == 0 {
		return c.HTML(http.StatusOK, "")
	}

	var peers []FleetPeer
	for _, p := range resp.Peers {
		if p.Hostname == h.cfg.Hostname || p.Address == "" {
			continue
		}
		port := p.Port
		if port == 0 {
			port = 8080
		}
		peers = append(peers, FleetPeer{
			Hostname: p.Hostname,
			Address:  p.Address,
			Port:     port,
			DashURL:  fmt.Sprintf("http://%s:%d", p.Address, port),
		})
	}

	if len(peers) == 0 {
		return c.HTML(http.StatusOK, "")
	}

	var buf strings.Builder
	buf.WriteString(`<article><header><strong>Fleet</strong></header><div class="peers-compact">`)
	for _, p := range peers {
		fmt.Fprintf(&buf, `<a href="%s" target="_blank" rel="noopener" class="peer-chip"><span class="peer-dot"></span>%s`,
			html.EscapeString(p.DashURL), html.EscapeString(p.Hostname))
		buf.WriteString(`</a>`)
	}
	buf.WriteString(`</div>`)
	buf.WriteString(`<footer><a href="/fleet">Fleet details &rarr;</a></footer></article>`)

	return c.HTML(http.StatusOK, buf.String())
}

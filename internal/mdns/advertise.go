package mdns

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/grandcat/zeroconf"

	"github.com/jdillenberger/homelabctl/internal/config"
)

const serviceType = "_homelabctl._tcp"

// Advertise registers the homelabctl service via mDNS and returns a shutdown function.
// It should be run as part of serve mode; the returned function stops advertising.
func Advertise(cfg *config.Config, version string, apps []string) (shutdown func(), err error) {
	hostname := cfg.Hostname
	port := cfg.Network.WebPort

	// Build TXT records
	txt := []string{
		"hostname=" + hostname,
		"version=" + version,
		"apps=" + strings.Join(apps, ","),
		"role=primary",
	}

	server, err := zeroconf.Register(
		hostname,    // instance name
		serviceType, // service type
		"local.",    // domain
		port,        // port
		txt,         // TXT records
		nil,         // interfaces (nil = all)
	)
	if err != nil {
		return nil, fmt.Errorf("registering mDNS service: %w", err)
	}

	slog.Debug("mDNS advertising started", "hostname", hostname, "port", port)

	return func() {
		server.Shutdown()
		slog.Debug("mDNS advertising stopped")
	}, nil
}

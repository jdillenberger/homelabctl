package mdns

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/grandcat/zeroconf"

	"github.com/jdillenberger/homelabctl/internal/config"
)

const serviceType = "_homelabctl._tcp"

// Advertise registers the homelabctl service via mDNS and returns a shutdown function.
// It should be run as part of daemon mode; the returned function stops advertising.
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

	ifaces, err := physicalInterfaces()
	if err != nil {
		return nil, fmt.Errorf("detecting network interfaces: %w", err)
	}

	server, err := zeroconf.Register(
		hostname,    // instance name
		serviceType, // service type
		"local.",    // domain
		port,        // port
		txt,         // TXT records
		ifaces,      // interfaces (physical only)
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

// physicalInterfaces returns non-virtual network interfaces, excluding
// Docker bridges, veth pairs, and loopback.
func physicalInterfaces() ([]net.Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []net.Interface
	for _, iface := range all {
		name := iface.Name
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "veth") {
			continue
		}
		result = append(result, iface)
	}
	return result, nil
}

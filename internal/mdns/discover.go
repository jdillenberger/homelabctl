package mdns

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"

	"github.com/jdillenberger/homelabctl/internal/config"
)

const defaultDiscoverTimeout = 5 * time.Second

// Discover browses for homelabctl mDNS services and returns discovered fleet hosts.
// If timeout is 0, it defaults to 5 seconds.
func Discover(timeout time.Duration) ([]config.FleetHost, error) {
	if timeout == 0 {
		timeout = defaultDiscoverTimeout
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("creating mDNS resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	var hosts []config.FleetHost

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Collect results in background
	done := make(chan struct{})
	go func() {
		for entry := range entries {
			host := parseServiceEntry(entry)
			hosts = append(hosts, host)
		}
		close(done)
	}()

	if err := resolver.Browse(ctx, serviceType, "local.", entries); err != nil {
		return nil, fmt.Errorf("browsing mDNS services: %w", err)
	}

	// Wait for browsing to complete.
	// The zeroconf resolver closes the entries channel when the context
	// expires, so we must NOT close it ourselves (double-close panics).
	<-ctx.Done()
	<-done

	return hosts, nil
}

// parseServiceEntry converts a zeroconf entry into a FleetHost.
func parseServiceEntry(entry *zeroconf.ServiceEntry) config.FleetHost {
	host := config.FleetHost{
		Hostname: entry.Instance,
		Port:     entry.Port,
		Online:   true,
	}

	// Use first IPv4 address if available, fall back to IPv6
	if len(entry.AddrIPv4) > 0 {
		host.Address = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		host.Address = entry.AddrIPv6[0].String()
	}

	// Parse TXT records
	for _, txt := range entry.Text {
		key, value, ok := strings.Cut(txt, "=")
		if !ok {
			continue
		}
		switch key {
		case "hostname":
			host.Hostname = value
		case "version":
			host.Version = value
		case "apps":
			if value != "" {
				host.Apps = strings.Split(value, ",")
			}
		case "role":
			host.Role = value
		case "port":
			if p, err := strconv.Atoi(value); err == nil {
				host.Port = p
			}
		}
	}

	return host
}

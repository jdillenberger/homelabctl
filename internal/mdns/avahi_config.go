package mdns

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

const avahiConfigPath = "/etc/avahi/avahi-daemon.conf"

// virtualPrefixes lists network interface name prefixes that are virtual/container-related.
var virtualPrefixes = []string{
	"docker", // Docker default bridge
	"br-",    // Docker user-defined bridges
	"veth",   // Docker container veth pairs
	"virbr",  // libvirt
	"tun",    // VPN tunnels
	"tap",    // TAP devices
}

// EnsureAvahiConfig makes sure avahi-daemon.conf restricts mDNS to physical
// network interfaces so that Docker bridge interfaces don't cause hostname
// resolution to return container-network IPs.
//
// The function is idempotent: it skips if avahi-daemon is not installed or
// allow-interfaces is already configured. Errors are non-fatal (logged to stderr).
func EnsureAvahiConfig() {
	// Check if avahi-daemon is installed.
	if _, err := exec.LookPath("avahi-daemon"); err != nil {
		return
	}

	data, err := os.ReadFile(avahiConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "avahi: cannot read %s: %v\n", avahiConfigPath, err)
		return
	}

	// If allow-interfaces is already configured (uncommented), skip.
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "allow-interfaces=") {
			return
		}
	}

	ifaces, err := PhysicalInterfaces()
	if err != nil || len(ifaces) == 0 {
		fmt.Fprintf(os.Stderr, "avahi: cannot detect physical interfaces: %v\n", err)
		return
	}

	// Replace the commented-out allow-interfaces line or add it after [server].
	content := string(data)
	ifaceList := strings.Join(ifaces, ",")
	directive := "allow-interfaces=" + ifaceList

	if strings.Contains(content, "#allow-interfaces=") {
		content = strings.Replace(content, "#allow-interfaces=eth0", directive, 1)
	} else {
		content = strings.Replace(content, "[server]\n", "[server]\n"+directive+"\n", 1)
	}

	if err := os.WriteFile(avahiConfigPath, []byte(content), 0o644); err != nil {
		if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "avahi: cannot write %s (permission denied). Run with sudo or manually set:\n  %s\n", avahiConfigPath, directive)
		} else {
			fmt.Fprintf(os.Stderr, "avahi: cannot write %s: %v\n", avahiConfigPath, err)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "avahi: configured allow-interfaces=%s\n", ifaceList)

	// Restart avahi-daemon to apply.
	if out, err := exec.Command("systemctl", "restart", "avahi-daemon").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "avahi: restart failed: %v: %s\n", err, out)
	}
}

// PhysicalInterfaces returns the names of non-virtual, non-loopback network interfaces.
func PhysicalInterfaces() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var physical []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtual(iface.Name) {
			continue
		}
		physical = append(physical, iface.Name)
	}
	return physical, nil
}

func isVirtual(name string) bool {
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

package mdns

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"

	homelabExec "github.com/jdillenberger/homelabctl/internal/exec"
)

// AvahiCNAME manages avahi-publish processes for app domains.
type AvahiCNAME struct {
	runner    *homelabExec.Runner
	mu        sync.Mutex
	processes map[string]*os.Process // key -> running process
	localIP   string
}

// NewAvahiCNAME creates a new AvahiCNAME manager.
func NewAvahiCNAME(runner *homelabExec.Runner) *AvahiCNAME {
	return &AvahiCNAME{
		runner:    runner,
		processes: make(map[string]*os.Process),
		localIP:   detectLocalIP(),
	}
}

// PublishCNAME publishes a domain name via mDNS using avahi-publish.
// It uses avahi-publish -a to create an address record pointing to the local IP.
func (a *AvahiCNAME) PublishCNAME(key, domain string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.processes[key]; exists {
		return fmt.Errorf("already published: %s", key)
	}

	if a.localIP == "" {
		return fmt.Errorf("could not detect local IP address")
	}

	if a.runner.Verbose {
		fmt.Fprintf(os.Stderr, "avahi: publishing %s -> %s\n", domain, a.localIP)
	}

	// Use avahi-publish -a -R to publish an address record.
	// -R skips the reverse (PTR) record to avoid conflicts.
	cmd := exec.Command("avahi-publish", "-a", "-R", domain, a.localIP)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting avahi-publish for %s: %w", domain, err)
	}

	a.processes[key] = cmd.Process

	// Reap the process in the background so it doesn't become a zombie
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		delete(a.processes, key)
		a.mu.Unlock()
	}()

	return nil
}

// UnpublishCNAME kills the avahi-publish process for the given key.
func (a *AvahiCNAME) UnpublishCNAME(key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	proc, exists := a.processes[key]
	if !exists {
		return fmt.Errorf("no record published for %s", key)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("killing avahi process for %s: %w", key, err)
	}

	delete(a.processes, key)

	if a.runner.Verbose {
		fmt.Fprintf(os.Stderr, "avahi: unpublished %s\n", key)
	}

	return nil
}

// ListPublished returns a map of published keys.
func (a *AvahiCNAME) ListPublished() map[string]bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make(map[string]bool, len(a.processes))
	for name := range a.processes {
		result[name] = true
	}
	return result
}

// Shutdown kills all running avahi-publish processes.
func (a *AvahiCNAME) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for name, proc := range a.processes {
		_ = proc.Kill()
		if a.runner.Verbose {
			fmt.Fprintf(os.Stderr, "avahi: killed process for %s\n", name)
		}
	}
	a.processes = make(map[string]*os.Process)
}

// detectLocalIP returns the primary non-loopback IPv4 address.
func detectLocalIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}

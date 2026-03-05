package mdns

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	homelabExec "github.com/jdillenberger/homelabctl/internal/exec"
)

// AvahiCNAME manages avahi-publish-cname-on-all processes for app domains.
type AvahiCNAME struct {
	runner    *homelabExec.Runner
	mu        sync.Mutex
	processes map[string]*os.Process // appName -> running process
}

// NewAvahiCNAME creates a new AvahiCNAME manager.
func NewAvahiCNAME(runner *homelabExec.Runner) *AvahiCNAME {
	return &AvahiCNAME{
		runner:    runner,
		processes: make(map[string]*os.Process),
	}
}

// PublishCNAME starts a background avahi-publish-cname-on-all process for the given app.
// The CNAME will resolve to <domain> (e.g. "myapp.local").
func (a *AvahiCNAME) PublishCNAME(appName, domain string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Don't start a duplicate
	if _, exists := a.processes[appName]; exists {
		return fmt.Errorf("CNAME for %s is already published", appName)
	}

	if a.runner.Verbose {
		fmt.Fprintf(os.Stderr, "avahi: publishing CNAME %s\n", domain)
	}

	cmd := exec.Command("avahi-publish-cname-on-all", domain)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting avahi-publish-cname-on-all for %s: %w", appName, err)
	}

	a.processes[appName] = cmd.Process

	// Reap the process in the background so it doesn't become a zombie
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		delete(a.processes, appName)
		a.mu.Unlock()
	}()

	return nil
}

// UnpublishCNAME kills the avahi-publish process for the given app.
func (a *AvahiCNAME) UnpublishCNAME(appName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	proc, exists := a.processes[appName]
	if !exists {
		return fmt.Errorf("no CNAME published for %s", appName)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("killing avahi process for %s: %w", appName, err)
	}

	delete(a.processes, appName)

	if a.runner.Verbose {
		fmt.Fprintf(os.Stderr, "avahi: unpublished CNAME for %s\n", appName)
	}

	return nil
}

// ListPublished returns a map of app names to their published CNAME domains.
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
			fmt.Fprintf(os.Stderr, "avahi: killed CNAME process for %s\n", name)
		}
	}
	a.processes = make(map[string]*os.Process)
}

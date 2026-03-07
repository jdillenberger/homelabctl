package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jdillenberger/homelabctl/internal/mdns"
)

// CheckResult holds the result of a single dependency check.
type CheckResult struct {
	Name           string `json:"name"`
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	InstallCommand string `json:"install_command,omitempty"`
}

// Dependency defines a system dependency to check.
type Dependency struct {
	Name           string
	Binary         string   // binary name to look up with `which`
	VersionArgs    []string // args to get version (e.g. --version)
	InstallCommand string   // apt install command
}

// DefaultDependencies returns the list of dependencies to check.
func DefaultDependencies() []Dependency {
	return []Dependency{
		{
			Name:           "docker",
			Binary:         "docker",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y docker.io",
		},
		{
			Name:           "docker compose",
			Binary:         "docker",
			VersionArgs:    []string{"compose", "version"},
			InstallCommand: "apt install -y docker-compose-v2",
		},
		{
			Name:           "avahi-daemon",
			Binary:         "avahi-daemon",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y avahi-daemon",
		},
		{
			Name:           "avahi-utils",
			Binary:         "avahi-browse",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y avahi-utils",
		},
		{
			Name:           "borgmatic",
			Binary:         "borgmatic",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y borgmatic",
		},
		{
			Name:           "borg",
			Binary:         "borg",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y borgbackup",
		},
		{
			Name:           "libnss-mdns",
			Binary:         "getent",
			VersionArgs:    nil,
			InstallCommand: "apt install -y libnss-mdns",
		},
	}
}

// Check runs a single dependency check.
func Check(dep Dependency) CheckResult {
	result := CheckResult{
		Name:           dep.Name,
		InstallCommand: dep.InstallCommand,
	}

	// Check if binary exists
	path, err := exec.LookPath(dep.Binary)
	if err != nil {
		return result
	}

	result.Installed = true

	// Try to get version
	if len(dep.VersionArgs) > 0 {
		cmd := exec.Command(path, dep.VersionArgs...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			ver := strings.TrimSpace(string(out))
			// Take first line only
			if idx := strings.IndexByte(ver, '\n'); idx != -1 {
				ver = ver[:idx]
			}
			result.Version = ver
		}
	}

	return result
}

// CheckAll runs all default dependency checks and system checks.
func CheckAll() []CheckResult {
	deps := DefaultDependencies()
	results := make([]CheckResult, len(deps))
	for i, dep := range deps {
		results[i] = Check(dep)
	}
	results = append(results, CheckNSSwitchMDNS())
	results = append(results, CheckAvahiRunning())
	results = append(results, CheckAvahiInterfaces())
	results = append(results, CheckResolvedMDNS())
	results = append(results, CheckAvahiHostnameConflict())
	return results
}

// CheckNSSwitchMDNS checks that /etc/nsswitch.conf has mdns4 (not mdns4_minimal)
// in the hosts line, and that /etc/mdns.allow is configured for routing domains.
func CheckNSSwitchMDNS() CheckResult {
	result := CheckResult{
		Name:           "nsswitch-mdns",
		InstallCommand: "",
	}

	data, err := os.ReadFile("/etc/nsswitch.conf")
	if err != nil {
		result.Version = "cannot read /etc/nsswitch.conf"
		return result
	}

	hasMDNS4 := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "hosts:") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if f == "mdns4" || f == "mdns" {
					hasMDNS4 = true
				}
			}
			if !hasMDNS4 {
				if strings.Contains(line, "mdns4_minimal") {
					result.Version = "mdns4_minimal configured (only single-label .local — run doctor --fix)"
				} else {
					result.Version = "mdns NOT configured in hosts line"
				}
				return result
			}
			break
		}
	}

	if !hasMDNS4 {
		result.Version = "no hosts line found"
		return result
	}

	// Also check /etc/mdns.allow — without it, nss-mdns only resolves 2-label .local names
	allowData, err := os.ReadFile("/etc/mdns.allow")
	if err != nil {
		result.Version = "mdns4 in nsswitch but /etc/mdns.allow missing (run doctor --fix)"
		return result
	}
	if strings.Contains(string(allowData), ".local") {
		result.Installed = true
		result.Version = "mdns4 configured, mdns.allow present"
	} else {
		result.Version = "mdns4 in nsswitch but /etc/mdns.allow missing .local entry (run doctor --fix)"
	}
	return result
}

// CheckAvahiRunning checks if avahi-daemon is active.
func CheckAvahiRunning() CheckResult {
	result := CheckResult{
		Name:           "avahi-daemon-running",
		InstallCommand: "systemctl enable --now avahi-daemon",
	}

	cmd := exec.Command("systemctl", "is-active", "avahi-daemon")
	out, err := cmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		result.Installed = true
		result.Version = "active"
	} else {
		result.Version = strings.TrimSpace(string(out))
	}
	return result
}

// CheckAvahiInterfaces verifies that avahi-daemon.conf restricts mDNS to
// physical interfaces so Docker bridge interfaces don't hijack .local resolution.
func CheckAvahiInterfaces() CheckResult {
	result := CheckResult{
		Name: "avahi-interfaces",
	}

	if _, err := exec.LookPath("avahi-daemon"); err != nil {
		result.Version = "avahi-daemon not installed, skipped"
		result.Installed = true
		return result
	}

	data, err := os.ReadFile("/etc/avahi/avahi-daemon.conf")
	if err != nil {
		result.Version = "cannot read /etc/avahi/avahi-daemon.conf"
		return result
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "allow-interfaces=") {
			result.Installed = true
			result.Version = strings.TrimSpace(line)
			return result
		}
	}

	ifaces, _ := mdns.PhysicalInterfaces()
	result.Version = fmt.Sprintf("allow-interfaces not set — Docker interfaces may hijack .local resolution (run doctor --fix to set allow-interfaces=%s)", strings.Join(ifaces, ","))
	return result
}

// Fix attempts to install a missing dependency using apt or fix system config.
func Fix(result CheckResult) error {
	if result.Installed {
		return nil
	}

	// Special handler for nsswitch-mdns
	if result.Name == "nsswitch-mdns" {
		return fixNSSwitchMDNS()
	}

	// Special handler for avahi-interfaces
	if result.Name == "avahi-interfaces" {
		return fixAvahiInterfaces()
	}

	// Special handler for resolved-mdns
	if result.Name == "resolved-mdns" {
		return fixResolvedMDNS()
	}

	if result.InstallCommand == "" {
		return fmt.Errorf("no install command for %s", result.Name)
	}

	parts := strings.Fields(result.InstallCommand)
	cmd := exec.Command("sudo", parts...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installing %s: %w\n%s", result.Name, err, string(out))
	}
	return nil
}

// fixNSSwitchMDNS ensures:
// 1. /etc/nsswitch.conf hosts line uses mdns4 (not mdns4_minimal)
// 2. /etc/mdns.allow exists with .local to allow multi-label .local resolution
func fixNSSwitchMDNS() error {
	// Step 1: Fix nsswitch.conf
	data, err := os.ReadFile("/etc/nsswitch.conf")
	if err != nil {
		return fmt.Errorf("reading /etc/nsswitch.conf: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	nssModified := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "hosts:") {
			if strings.Contains(trimmed, "mdns4_minimal") {
				lines[i] = strings.Replace(line, "mdns4_minimal", "mdns4", 1)
				nssModified = true
			} else if !strings.Contains(trimmed, "mdns4") && !strings.Contains(trimmed, "mdns") {
				lines[i] = strings.Replace(line, "dns", "mdns4 [NOTFOUND=return] dns", 1)
				nssModified = true
			}
			break
		}
	}

	if nssModified {
		tmpFile := "/tmp/nsswitch.conf.homelabctl"
		if err := os.WriteFile(tmpFile, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return fmt.Errorf("writing temp file: %w", err)
		}
		cmd := exec.Command("sudo", "cp", tmpFile, "/etc/nsswitch.conf")
		out, err := cmd.CombinedOutput()
		os.Remove(tmpFile)
		if err != nil {
			return fmt.Errorf("updating /etc/nsswitch.conf: %w\n%s", err, string(out))
		}
		fmt.Println("    Updated /etc/nsswitch.conf: mdns4_minimal -> mdns4")
	}

	// Step 2: Create /etc/mdns.allow if missing or incomplete
	allowData, _ := os.ReadFile("/etc/mdns.allow")
	if !strings.Contains(string(allowData), ".local") {
		// Write mdns.allow with .local to allow all *.local multi-label resolution
		tmpFile := "/tmp/mdns.allow.homelabctl"
		content := ".local\n.local.\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing temp file: %w", err)
		}
		cmd := exec.Command("sudo", "cp", tmpFile, "/etc/mdns.allow")
		out, err := cmd.CombinedOutput()
		os.Remove(tmpFile)
		if err != nil {
			return fmt.Errorf("creating /etc/mdns.allow: %w\n%s", err, string(out))
		}
		fmt.Println("    Created /etc/mdns.allow with .local domain")
	}

	return nil
}

// CheckResolvedMDNS checks if systemd-resolved has mDNS enabled globally.
// Without this, .local name resolution fails even when avahi-daemon is running.
func CheckResolvedMDNS() CheckResult {
	result := CheckResult{
		Name: "resolved-mdns",
	}

	// Check if systemd-resolved is active
	cmd := exec.Command("systemctl", "is-active", "systemd-resolved")
	out, _ := cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "active" {
		result.Installed = true
		result.Version = "systemd-resolved not active, skipped"
		return result
	}

	// Check global mDNS setting via resolvectl
	cmd = exec.Command("resolvectl", "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Version = "cannot query resolvectl status"
		return result
	}

	// Look for the global Protocols line: "+mDNS" means enabled, "-mDNS" means disabled
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Protocols:") {
			if strings.Contains(trimmed, "+mDNS") {
				result.Installed = true
				result.Version = "mDNS enabled globally"
			} else if strings.Contains(trimmed, "-mDNS") {
				result.Version = "mDNS disabled globally in systemd-resolved (run doctor --fix)"
			}
			return result
		}
	}

	result.Version = "could not determine mDNS status"
	return result
}

// fixAvahiInterfaces sets allow-interfaces in avahi-daemon.conf to physical
// interfaces only, preventing Docker bridges from hijacking .local resolution.
func fixAvahiInterfaces() error {
	ifaces, err := mdns.PhysicalInterfaces()
	if err != nil || len(ifaces) == 0 {
		return fmt.Errorf("detecting physical interfaces: %w", err)
	}

	data, err := os.ReadFile("/etc/avahi/avahi-daemon.conf")
	if err != nil {
		return fmt.Errorf("reading /etc/avahi/avahi-daemon.conf: %w", err)
	}

	content := string(data)
	ifaceList := strings.Join(ifaces, ",")
	directive := "allow-interfaces=" + ifaceList

	if strings.Contains(content, "#allow-interfaces=") {
		content = strings.Replace(content, "#allow-interfaces=eth0", directive, 1)
	} else {
		content = strings.Replace(content, "[server]\n", "[server]\n"+directive+"\n", 1)
	}

	tmpFile := "/tmp/avahi-daemon.conf.homelabctl"
	if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	cmd := exec.Command("sudo", "cp", tmpFile, "/etc/avahi/avahi-daemon.conf")
	out, err := cmd.CombinedOutput()
	os.Remove(tmpFile)
	if err != nil {
		return fmt.Errorf("updating /etc/avahi/avahi-daemon.conf: %w\n%s", err, string(out))
	}

	fmt.Printf("    Set %s in /etc/avahi/avahi-daemon.conf\n", directive)

	// Restart avahi-daemon to apply.
	cmd = exec.Command("sudo", "systemctl", "restart", "avahi-daemon")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restarting avahi-daemon: %w\n%s", err, string(out))
	}

	fmt.Println("    Restarted avahi-daemon")
	return nil
}

// CheckAvahiHostnameConflict detects if avahi-daemon has renamed the hostname
// due to mDNS conflicts (e.g. x1.local -> x1-2.local), typically caused by
// orphaned avahi-publish processes from previous daemon runs.
func CheckAvahiHostnameConflict() CheckResult {
	result := CheckResult{
		Name: "avahi-hostname-conflict",
	}

	hostname, err := os.Hostname()
	if err != nil {
		result.Version = "cannot determine hostname"
		return result
	}

	// Try to resolve our hostname via avahi
	cmd := exec.Command("avahi-resolve", "-n", hostname+".local")
	out, err := cmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		result.Installed = true
		result.Version = hostname + ".local resolves OK"
		return result
	}

	// Resolution failed — check journal for conflict messages
	cmd = exec.Command("journalctl", "-u", "avahi-daemon", "--since", "1 hour ago", "--no-pager", "-q")
	out, _ = cmd.CombinedOutput()
	if strings.Contains(string(out), "Host name conflict") {
		result.Version = hostname + ".local has hostname conflict — kill orphaned avahi-publish processes and restart avahi-daemon"
		return result
	}

	result.Version = hostname + ".local not resolvable via mDNS"
	return result
}

// fixResolvedMDNS enables mDNS globally in systemd-resolved via a drop-in config.
func fixResolvedMDNS() error {
	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := dropInDir + "/homelabctl-mdns.conf"
	content := "[Resolve]\nMulticastDNS=resolve\n"

	// Create drop-in directory
	cmd := exec.Command("sudo", "mkdir", "-p", dropInDir)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating %s: %w", dropInDir, err)
	}

	// Write drop-in config via sudo tee
	cmd = exec.Command("sudo", "tee", dropInFile)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stderr = os.Stderr
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing %s: %w", dropInFile, err)
	}
	fmt.Printf("    Created %s\n", dropInFile)

	// Restart systemd-resolved to apply
	cmd = exec.Command("sudo", "systemctl", "restart", "systemd-resolved")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restarting systemd-resolved: %w", err)
	}
	fmt.Println("    Restarted systemd-resolved")

	return nil
}

package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	results = append(results, CheckTraefikMDNS())
	results = append(results, CheckPeerScanner())
	return results
}

// Fix attempts to install a missing dependency using apt or fix system config.
func Fix(result CheckResult) error {
	if result.Installed {
		return nil
	}

	if result.Name == "traefik-mdns" {
		return fixTraefikMDNS()
	}

	if result.Name == "peer-scanner" {
		return fixPeerScanner()
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

// CheckTraefikMDNS checks if traefik-mdns binary is installed and its service is running.
// This is an optional check — traefik-mdns provides .local domain publishing for Docker
// containers with Traefik labels.
func CheckTraefikMDNS() CheckResult {
	result := CheckResult{
		Name: "traefik-mdns",
	}

	// Check if binary is installed
	path, err := exec.LookPath("traefik-mdns")
	if err != nil {
		result.Version = "not installed (optional — install: curl -fsSL https://raw.githubusercontent.com/komphost/traefik-mdns/main/install.sh | sudo bash)"
		return result
	}

	// Get version
	cmd := exec.Command(path, "version")
	out, err := cmd.CombinedOutput()
	if err == nil {
		result.Version = strings.TrimSpace(string(out))
	}

	// Check if service is running
	cmd = exec.Command("systemctl", "is-active", "traefik-mdns")
	out, _ = cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) == "active" {
		result.Installed = true
		if result.Version != "" {
			result.Version += " (service active)"
		} else {
			result.Version = "service active"
		}
	} else {
		result.Version += " (service not running — run: sudo traefik-mdns setup)"
	}

	return result
}

// CheckPeerScanner checks if peer-scanner binary is installed and its service is running.
func CheckPeerScanner() CheckResult {
	result := CheckResult{
		Name: "peer-scanner",
	}

	// Check if binary is installed
	path, err := exec.LookPath("peer-scanner")
	if err != nil {
		result.Version = "not installed (optional — install: curl -fsSL https://raw.githubusercontent.com/komphost/peer-scanner/main/install.sh | sudo bash)"
		return result
	}

	// Get version
	cmd := exec.Command(path, "version")
	out, err := cmd.CombinedOutput()
	if err == nil {
		result.Version = strings.TrimSpace(string(out))
	}

	// Check if service is running
	cmd = exec.Command("systemctl", "is-active", "peer-scanner")
	out, _ = cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) == "active" {
		result.Installed = true
		if result.Version != "" {
			result.Version += " (service active)"
		} else {
			result.Version = "service active"
		}
	} else {
		result.Version += " (service not running — run: sudo peer-scanner setup)"
	}

	return result
}

// fixPeerScanner installs peer-scanner and sets up its service.
func fixPeerScanner() error {
	// Install binary
	fmt.Println("    Installing peer-scanner...")
	cmd := exec.Command("bash", "-c", "curl -fsSL https://raw.githubusercontent.com/komphost/peer-scanner/main/install.sh | bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing peer-scanner: %w", err)
	}

	// Run setup
	fmt.Println("    Running peer-scanner setup...")
	cmd = exec.Command("peer-scanner", "setup")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running peer-scanner setup: %w", err)
	}

	return nil
}

// fixTraefikMDNS installs traefik-mdns and sets up its service.
func fixTraefikMDNS() error {
	// Install binary
	fmt.Println("    Installing traefik-mdns...")
	cmd := exec.Command("bash", "-c", "curl -fsSL https://raw.githubusercontent.com/komphost/traefik-mdns/main/install.sh | bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing traefik-mdns: %w", err)
	}

	// Run setup (doctor --fix + service install)
	fmt.Println("    Running traefik-mdns setup...")
	cmd = exec.Command("traefik-mdns", "setup")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running traefik-mdns setup: %w", err)
	}

	return nil
}

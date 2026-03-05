package doctor

import (
	"fmt"
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

// CheckAll runs all default dependency checks.
func CheckAll() []CheckResult {
	deps := DefaultDependencies()
	results := make([]CheckResult, len(deps))
	for i, dep := range deps {
		results[i] = Check(dep)
	}
	return results
}

// Fix attempts to install a missing dependency using apt.
func Fix(result CheckResult) error {
	if result.Installed {
		return nil
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

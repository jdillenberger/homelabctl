package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
)

var validComponents = []string{"dashboard", "scheduler"}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the homelabctl systemd service",
	Long:  "Install, uninstall, or check status of the homelabctl systemd service.",
}

func homelabctlBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "/usr/local/bin/homelabctl"
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

// serviceUnitName returns the systemd unit name for the given component.
// An empty component means the combined daemon.
func serviceUnitName(component string) string {
	if component == "" {
		return "homelabctl.service"
	}
	return fmt.Sprintf("homelabctl-%s.service", component)
}

// serviceUnitPath returns the systemd unit file path for the given component.
func serviceUnitPath(component string) string {
	return "/etc/systemd/system/" + serviceUnitName(component)
}

// serviceDescription returns the unit description for the given component.
func serviceDescription(component string) string {
	switch component {
	case "dashboard":
		return "homelabctl web dashboard"
	case "scheduler":
		return "homelabctl background scheduler"
	default:
		return "homelabctl management daemon"
	}
}

func generateUnitFile(runtime, component string) string {
	binPath := homelabctlBinary()
	execStart := binPath + " daemon"
	if component != "" {
		execStart += " " + component
	}
	return fmt.Sprintf(`[Unit]
Description=%s
After=network-online.target %s.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, serviceDescription(component), runtime, execStart)
}

func validateComponent(component string) error {
	if component == "" {
		return nil
	}
	for _, v := range validComponents {
		if component == v {
			return nil
		}
	}
	return fmt.Errorf("invalid component %q, valid components: %v", component, validComponents)
}

var serviceInstallCmd = &cobra.Command{
	Use:       "install [component]",
	Short:     "Install the homelabctl systemd service",
	Long:      "Install a systemd service. Optionally specify a component (dashboard, scheduler).",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: validComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		var component string
		if len(args) > 0 {
			component = args[0]
		}
		if err := validateComponent(component); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		unit := generateUnitFile(cfg.Docker.Runtime, component)
		unitPath := serviceUnitPath(component)
		unitName := serviceUnitName(component)

		if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
			return fmt.Errorf("writing unit file: %w (are you root?)", err)
		}
		fmt.Printf("Unit file written to %s\n", unitPath)

		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}
		if err := runSystemctl("enable", unitName); err != nil {
			return err
		}
		if err := runSystemctl("start", unitName); err != nil {
			return err
		}

		fmt.Println("Service installed and started.")
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:       "uninstall [component]",
	Short:     "Uninstall the homelabctl systemd service",
	Long:      "Uninstall a systemd service. Optionally specify a component (dashboard, scheduler).",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: validComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		var component string
		if len(args) > 0 {
			component = args[0]
		}
		if err := validateComponent(component); err != nil {
			return err
		}

		unitName := serviceUnitName(component)
		unitPath := serviceUnitPath(component)

		_ = runSystemctl("stop", unitName)
		_ = runSystemctl("disable", unitName)

		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing unit file: %w", err)
		}

		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}

		fmt.Println("Service uninstalled.")
		return nil
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:       "status [component]",
	Short:     "Show homelabctl service status",
	Long:      "Show systemd service status. Optionally specify a component (dashboard, scheduler).",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: validComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		var component string
		if len(args) > 0 {
			component = args[0]
		}
		if err := validateComponent(component); err != nil {
			return err
		}

		c := exec.Command("systemctl", "status", serviceUnitName(component))
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run()
		return nil
	},
}

func runSystemctl(args ...string) error {
	c := exec.Command("systemctl", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("systemctl %v: %w", args, err)
	}
	return nil
}

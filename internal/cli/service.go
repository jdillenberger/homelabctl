package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	unitName = "homelabctl.service"
	unitPath = "/etc/systemd/system/" + unitName
)

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

func generateUnitFile() string {
	binPath := homelabctlBinary()
	return fmt.Sprintf(`[Unit]
Description=homelabctl management daemon
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s serve
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, binPath)
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the homelabctl systemd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		unit := generateUnitFile()

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
	Use:   "uninstall",
	Short: "Uninstall the homelabctl systemd service",
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Use:   "status",
	Short: "Show homelabctl service status",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := exec.Command("systemctl", "status", unitName)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run() // systemctl status exits non-zero if not running
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

package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/wizard"
	"github.com/jdillenberger/homelabctl/templates"
)

func newManager() (*app.Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	runner := &exec.Runner{Verbose: verbose}
	return app.NewManager(cfg, runner, templates.FS)
}

func init() {
	rootCmd.AddCommand(appsCmd)
	appsCmd.AddCommand(appsListCmd)
	appsCmd.AddCommand(appsDeployCmd)
	appsCmd.AddCommand(appsRemoveCmd)
	appsCmd.AddCommand(appsStatusCmd)
	appsCmd.AddCommand(appsStartCmd)
	appsCmd.AddCommand(appsStopCmd)
	appsCmd.AddCommand(appsRestartCmd)
	appsCmd.AddCommand(appsLogsCmd)
	appsCmd.AddCommand(appsUpdateCmd)

	appsListCmd.Flags().Bool("all", false, "Show all available templates (not just deployed)")
	appsDeployCmd.Flags().StringP("values", "f", "", "YAML file with template values")
	appsDeployCmd.Flags().StringSlice("set", nil, "Set values (key=value)")
	appsDeployCmd.Flags().Bool("dry-run", false, "Show rendered files without deploying")
	appsDeployCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	appsRemoveCmd.Flags().Bool("keep-data", false, "Keep app data volumes")
	appsLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	appsLogsCmd.Flags().IntP("lines", "n", 100, "Number of lines to show")

	// Top-level "logs" shortcut (alias for "apps logs")
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().IntP("lines", "n", 100, "Number of lines to show")
}

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Manage apps",
	Long:  "Deploy, remove, and manage homelab applications.",
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List apps",
	Long:  "List deployed apps. Use --all to include available templates.",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		showAll, _ := cmd.Flags().GetBool("all")

		if showAll {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCATEGORY\tDESCRIPTION\tSTATUS")
			deployed, _ := mgr.ListDeployed()
			deployedSet := make(map[string]bool)
			for _, d := range deployed {
				deployedSet[d] = true
			}
			for _, meta := range mgr.Registry().All() {
				status := "available"
				if deployedSet[meta.Name] {
					status = "deployed"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", meta.Name, meta.Category, meta.Description, status)
			}
			w.Flush()
			return nil
		}

		deployed, err := mgr.ListDeployed()
		if err != nil {
			return err
		}

		if len(deployed) == 0 {
			fmt.Println("No apps deployed. Use 'homelabctl apps list --all' to see available templates.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tDEPLOYED")
		for _, name := range deployed {
			info, err := mgr.GetDeployedInfo(name)
			if err != nil {
				fmt.Fprintf(w, "%s\t\terror reading info\n", name)
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", info.Name, info.Version, info.DeployedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

var appsDeployCmd = &cobra.Command{
	Use:   "deploy <app>",
	Short: "Deploy an app",
	Long:  "Deploy an app from a built-in template.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		appName := args[0]
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Parse --values / -f file
		values := make(map[string]string)
		valuesFile, _ := cmd.Flags().GetString("values")
		if valuesFile != "" {
			data, err := os.ReadFile(valuesFile)
			if err != nil {
				return fmt.Errorf("reading values file: %w", err)
			}
			if err := yaml.Unmarshal(data, &values); err != nil {
				return fmt.Errorf("parsing values file: %w", err)
			}
		}

		// Parse --set values (takes precedence over file values)
		setValues, _ := cmd.Flags().GetStringSlice("set")
		for _, kv := range setValues {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --set value: %q (expected key=value)", kv)
			}
			values[parts[0]] = parts[1]
		}

		// If no values provided and stdin is a terminal, offer the interactive wizard
		if len(values) == 0 && !dryRun {
			if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
				meta, ok := mgr.Registry().Get(appName)
				if ok && len(meta.Values) > 0 {
					wizardValues, err := wizard.RunDeployWizard(meta)
					if err != nil {
						return err
					}
					values = wizardValues
				}
			}
		}

		yes, _ := cmd.Flags().GetBool("yes")
		return mgr.Deploy(appName, app.DeployOptions{
			Values:  values,
			DryRun:  dryRun,
			Confirm: yes,
		})
	},
}

var appsRemoveCmd = &cobra.Command{
	Use:   "remove <app>",
	Short: "Remove a deployed app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}
		keepData, _ := cmd.Flags().GetBool("keep-data")
		return mgr.Remove(args[0], keepData)
	},
}

var appsStatusCmd = &cobra.Command{
	Use:   "status [app]",
	Short: "Show app container status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			deployed, err := mgr.ListDeployed()
			if err != nil {
				return err
			}
			for _, name := range deployed {
				fmt.Printf("=== %s ===\n", name)
				status, err := mgr.Status(name)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					continue
				}
				fmt.Println(status)
			}
			return nil
		}

		status, err := mgr.Status(args[0])
		if err != nil {
			return err
		}
		fmt.Print(status)
		return nil
	},
}

var appsStartCmd = &cobra.Command{
	Use:   "start <app>",
	Short: "Start a deployed app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}
		return mgr.Start(args[0])
	},
}

var appsStopCmd = &cobra.Command{
	Use:   "stop <app>",
	Short: "Stop a deployed app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}
		return mgr.Stop(args[0])
	},
}

var appsRestartCmd = &cobra.Command{
	Use:   "restart <app>",
	Short: "Restart a deployed app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}
		return mgr.Restart(args[0])
	},
}

var appsLogsCmd = &cobra.Command{
	Use:   "logs <app>",
	Short: "Show app logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")
		return compose.Logs(cfg.AppDir(args[0]), os.Stdout, follow, lines)
	},
}

var appsUpdateCmd = &cobra.Command{
	Use:   "update <app>",
	Short: "Pull latest images and recreate containers",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
		appDir := cfg.AppDir(args[0])
		fmt.Printf("Pulling latest images for %s...\n", args[0])
		if _, err := compose.Pull(appDir); err != nil {
			return err
		}
		fmt.Printf("Recreating containers for %s...\n", args[0])
		if _, err := compose.Up(appDir); err != nil {
			return err
		}
		fmt.Printf("App %s updated.\n", args[0])
		return nil
	},
}

// logsCmd is a top-level shortcut for "apps logs".
var logsCmd = &cobra.Command{
	Use:   "logs <app>",
	Short: "Show app logs (shortcut for 'apps logs')",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")
		return compose.Logs(cfg.AppDir(args[0]), os.Stdout, follow, lines)
	},
}

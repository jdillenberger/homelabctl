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
	tmplFS := app.BuildTemplateFS(templates.FS, cfg.TemplatesDir)
	return app.NewManager(cfg, runner, tmplFS)
}

// completeTemplateNames returns available template names for shell completion.
func completeTemplateNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	mgr, err := newManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return mgr.Registry().List(), cobra.ShellCompDirectiveNoFileComp
}

// completeDeployedApps returns deployed app names for shell completion.
func completeDeployedApps(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	mgr, err := newManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	deployed, err := mgr.ListDeployed()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return deployed, cobra.ShellCompDirectiveNoFileComp
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
	appsCmd.AddCommand(appsInfoCmd)
	appsCmd.AddCommand(appsHealthCmd)
	appsCmd.AddCommand(appsPinCmd)
	appsPinCmd.Flags().Bool("dry-run", false, "Show what would be pinned without making changes")
	appsPinCmd.Flags().Bool("update", false, "Rewrite template files with pinned versions")
	appsPinCmd.ValidArgsFunction = completeTemplateNames

	appsCmd.AddCommand(appsOutdatedCmd)
	appsOutdatedCmd.ValidArgsFunction = completeDeployedApps

	// Dynamic completion: template names for deploy/info, deployed apps for the rest
	appsDeployCmd.ValidArgsFunction = completeTemplateNames
	appsInfoCmd.ValidArgsFunction = completeTemplateNames
	appsRemoveCmd.ValidArgsFunction = completeDeployedApps
	appsStatusCmd.ValidArgsFunction = completeDeployedApps
	appsStartCmd.ValidArgsFunction = completeDeployedApps
	appsStopCmd.ValidArgsFunction = completeDeployedApps
	appsRestartCmd.ValidArgsFunction = completeDeployedApps
	appsLogsCmd.ValidArgsFunction = completeDeployedApps
	appsUpdateCmd.ValidArgsFunction = completeDeployedApps

	appsListCmd.Flags().Bool("all", false, "Show all available templates (not just deployed)")
	appsListCmd.Flags().String("filter", "", "Filter apps by name or description substring")
	appsListCmd.Flags().String("category", "", "Filter apps by category")
	appsDeployCmd.Flags().StringP("values", "f", "", "YAML file with template values")
	appsDeployCmd.Flags().StringSlice("set", nil, "Set values (key=value)")
	appsDeployCmd.Flags().Bool("dry-run", false, "Show rendered files without deploying")
	appsDeployCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	appsRemoveCmd.Flags().Bool("keep-data", false, "Keep app data volumes")
	appsLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	appsLogsCmd.Flags().IntP("lines", "n", 100, "Number of lines to show")
	appsUpdateCmd.Flags().Bool("all", false, "Update all deployed apps")
	appsHealthCmd.ValidArgsFunction = completeDeployedApps

	// Top-level "logs" shortcut (alias for "apps logs")
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().IntP("lines", "n", 100, "Number of lines to show")
	logsCmd.ValidArgsFunction = completeDeployedApps
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
		filter, _ := cmd.Flags().GetString("filter")
		category, _ := cmd.Flags().GetString("category")
		filterLower := strings.ToLower(filter)

		if showAll {
			type appListEntry struct {
				Name        string `json:"name"`
				Category    string `json:"category"`
				Description string `json:"description"`
				Status      string `json:"status"`
			}

			deployed, _ := mgr.ListDeployed()
			deployedSet := make(map[string]bool)
			for _, d := range deployed {
				deployedSet[d] = true
			}

			var entries []appListEntry
			for _, meta := range mgr.Registry().All() {
				// Apply filters
				if filter != "" {
					nameLower := strings.ToLower(meta.Name)
					descLower := strings.ToLower(meta.Description)
					if !strings.Contains(nameLower, filterLower) && !strings.Contains(descLower, filterLower) {
						continue
					}
				}
				if category != "" && !strings.EqualFold(meta.Category, category) {
					continue
				}

				status := "available"
				if deployedSet[meta.Name] {
					status = "deployed"
				}
				entries = append(entries, appListEntry{
					Name:        meta.Name,
					Category:    meta.Category,
					Description: meta.Description,
					Status:      status,
				})
			}

			if jsonOutput {
				return outputJSON(entries)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCATEGORY\tDESCRIPTION\tSTATUS")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.Category, e.Description, e.Status)
			}
			w.Flush()
			return nil
		}

		deployed, err := mgr.ListDeployed()
		if err != nil {
			return err
		}

		if len(deployed) == 0 {
			if jsonOutput {
				return outputJSON([]struct{}{})
			}
			fmt.Println("No apps deployed. Use 'homelabctl apps list --all' to see available templates.")
			return nil
		}

		type deployedEntry struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			DeployedAt string `json:"deployed_at"`
		}

		var entries []deployedEntry
		for _, name := range deployed {
			// Apply filter to deployed apps too
			if filter != "" && !strings.Contains(strings.ToLower(name), filterLower) {
				continue
			}

			info, err := mgr.GetDeployedInfo(name)
			if err != nil {
				entries = append(entries, deployedEntry{Name: name, Version: "", DeployedAt: "error reading info"})
				continue
			}

			// Apply category filter if registry has the template
			if category != "" {
				if meta, ok := mgr.Registry().Get(name); ok {
					if !strings.EqualFold(meta.Category, category) {
						continue
					}
				}
			}

			entries = append(entries, deployedEntry{
				Name:       info.Name,
				Version:    info.Version,
				DeployedAt: info.DeployedAt.Format("2006-01-02 15:04"),
			})
		}

		if jsonOutput {
			return outputJSON(entries)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tDEPLOYED")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, e.Version, e.DeployedAt)
		}
		w.Flush()
		return nil
	},
}

var appsInfoCmd = &cobra.Command{
	Use:   "info <app>",
	Short: "Show app template details",
	Long:  "Display detailed information about an app template before deploying.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		meta, ok := mgr.Registry().Get(args[0])
		if !ok {
			return fmt.Errorf("unknown app template: %s", args[0])
		}

		if jsonOutput {
			return outputJSON(meta)
		}

		fmt.Printf("App: %s\n", meta.Name)
		fmt.Printf("Description: %s\n", meta.Description)
		fmt.Printf("Category: %s\n", meta.Category)
		fmt.Printf("Version: %s\n", meta.Version)

		if len(meta.Ports) > 0 {
			fmt.Println("\nPorts:")
			for _, p := range meta.Ports {
				fmt.Printf("  %d:%d/%s  %s\n", p.Host, p.Container, p.Protocol, p.Description)
			}
		}

		if len(meta.Volumes) > 0 {
			fmt.Println("\nVolumes:")
			for _, v := range meta.Volumes {
				fmt.Printf("  %-15s %s  (%s)\n", v.Name, v.Container, v.Description)
			}
		}

		if len(meta.Values) > 0 {
			fmt.Println("\nValues:")
			for _, v := range meta.Values {
				req := ""
				if v.Required {
					req = " [required]"
				}
				def := ""
				if v.Default != "" {
					def = fmt.Sprintf(" (default: %s)", v.Default)
				}
				secret := ""
				if v.Secret {
					secret = " [secret]"
				}
				autoGen := ""
				if v.AutoGen != "" {
					autoGen = fmt.Sprintf(" [auto: %s]", v.AutoGen)
				}
				fmt.Printf("  %-20s %s%s%s%s%s\n", v.Name, v.Description, def, req, secret, autoGen)
			}
		}

		if len(meta.Dependencies) > 0 {
			fmt.Printf("\nDependencies: %s\n", strings.Join(meta.Dependencies, ", "))
		}

		if meta.Requirements != nil {
			fmt.Println("\nRequirements:")
			if meta.Requirements.MinRAM != "" {
				fmt.Printf("  Min RAM:  %s\n", meta.Requirements.MinRAM)
			}
			if meta.Requirements.MinDisk != "" {
				fmt.Printf("  Min Disk: %s\n", meta.Requirements.MinDisk)
			}
			if len(meta.Requirements.Arch) > 0 {
				fmt.Printf("  Arch:     %s\n", strings.Join(meta.Requirements.Arch, ", "))
			}
		}

		if meta.HealthCheck != nil {
			fmt.Println("\nHealth Check:")
			fmt.Printf("  URL:      %s\n", meta.HealthCheck.URL)
			fmt.Printf("  Interval: %s\n", meta.HealthCheck.Interval)
		}

		if meta.Backup != nil {
			fmt.Println("\nBackup:")
			if len(meta.Backup.Paths) > 0 {
				fmt.Printf("  Paths: %s\n", strings.Join(meta.Backup.Paths, ", "))
			}
			if meta.Backup.PreHook != "" {
				fmt.Printf("  Pre-hook:  %s\n", meta.Backup.PreHook)
			}
			if meta.Backup.PostHook != "" {
				fmt.Printf("  Post-hook: %s\n", meta.Backup.PostHook)
			}
		}

		return nil
	},
}

var appsDeployCmd = &cobra.Command{
	Use:   "deploy <app>",
	Short: "Deploy an app",
	Long:  "Deploy an app from a built-in or local template.",
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

			if jsonOutput {
				type appStatus struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					Error  string `json:"error,omitempty"`
				}
				var statuses []appStatus
				for _, name := range deployed {
					status, err := mgr.Status(name)
					if err != nil {
						statuses = append(statuses, appStatus{Name: name, Error: err.Error()})
					} else {
						statuses = append(statuses, appStatus{Name: name, Status: status})
					}
				}
				return outputJSON(statuses)
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

		if jsonOutput {
			return outputJSON(map[string]string{
				"name":   args[0],
				"status": status,
			})
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
	Use:   "update [app]",
	Short: "Pull latest images and recreate containers",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)

		updateAll, _ := cmd.Flags().GetBool("all")

		if updateAll {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			deployed, err := mgr.ListDeployed()
			if err != nil {
				return err
			}
			if len(deployed) == 0 {
				fmt.Println("No apps deployed.")
				return nil
			}
			for _, appName := range deployed {
				appDir := cfg.AppDir(appName)
				fmt.Printf("Updating %s...\n", appName)
				if _, err := compose.Pull(appDir); err != nil {
					fmt.Printf("  Pull failed for %s: %v\n", appName, err)
					continue
				}
				if _, err := compose.Up(appDir); err != nil {
					fmt.Printf("  Recreate failed for %s: %v\n", appName, err)
					continue
				}
				fmt.Printf("  %s updated.\n", appName)
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("app name required (or use --all)")
		}

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

var appsHealthCmd = &cobra.Command{
	Use:   "health [app]",
	Short: "Check app health status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
		checker := app.NewHealthChecker()
		registry := mgr.Registry()

		var appsToCheck []string
		if len(args) > 0 {
			appsToCheck = []string{args[0]}
		} else {
			deployed, err := mgr.ListDeployed()
			if err != nil {
				return err
			}
			appsToCheck = deployed
		}

		if len(appsToCheck) == 0 {
			fmt.Println("No apps deployed.")
			return nil
		}

		type healthEntry struct {
			App    string `json:"app"`
			Status string `json:"status"`
			Detail string `json:"detail,omitempty"`
		}

		var results []healthEntry
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if !jsonOutput {
			fmt.Fprintln(w, "APP\tSTATUS\tDETAIL")
		}

		for _, appName := range appsToCheck {
			meta, ok := registry.Get(appName)
			if !ok {
				results = append(results, healthEntry{App: appName, Status: "unknown", Detail: "no template found"})
				if !jsonOutput {
					fmt.Fprintf(w, "%s\t%s\t%s\n", appName, "unknown", "no template found")
				}
				continue
			}

			r := checker.CheckApp(meta, compose, cfg.AppDir(appName))
			results = append(results, healthEntry{App: r.App, Status: string(r.Status), Detail: r.Detail})
			if !jsonOutput {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.App, r.Status, r.Detail)
			}
		}

		if jsonOutput {
			return outputJSON(results)
		}
		w.Flush()
		return nil
	},
}

var appsPinCmd = &cobra.Command{
	Use:   "pin [app]",
	Short: "Resolve floating image tags to pinned versions",
	Long:  "Scan templates for floating tags (latest, release) and resolve them via registry API to the highest semver tag with the same digest.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		tmplFS := app.BuildTemplateFS(templates.FS, cfg.TemplatesDir)

		entries, err := app.ScanFloatingTags(tmplFS)
		if err != nil {
			return fmt.Errorf("scanning templates: %w", err)
		}

		// Filter to specific app if provided
		if len(args) > 0 {
			appName := args[0]
			var filtered []app.FloatingTagEntry
			for _, e := range entries {
				if e.AppName == appName {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		if len(entries) == 0 {
			fmt.Println("No floating tags found.")
			return nil
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		update, _ := cmd.Flags().GetBool("update")

		resolver := app.NewImageResolver()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "APP\tIMAGE\tFLOATING\tPINNED\tSTATUS")

		for _, entry := range entries {
			result, err := resolver.ResolveFloatingTag(entry.Ref)
			if err != nil {
				fmt.Fprintf(w, "%s\t%s\t%s\t-\terror: %v\n", entry.AppName, entry.Image, entry.Ref.Tag, err)
				continue
			}

			if result.PinnedTag == "" {
				fmt.Fprintf(w, "%s\t%s\t%s\t-\tno semver tag found\n", entry.AppName, entry.Image, entry.Ref.Tag)
				continue
			}

			status := "found"
			if update && !dryRun {
				status = "updated"
				// Rewriting templates is left for a future enhancement;
				// for now we report what would change.
				status = "found (use --update to apply)"
			}
			if dryRun {
				status = "dry-run"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", entry.AppName, entry.Image, entry.Ref.Tag, result.PinnedTag, status)
		}
		w.Flush()
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

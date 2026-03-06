package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/templates"
)

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupInitCmd)
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupScheduleCmd)

	backupScheduleCmd.Flags().String("cron", "", "Set backup cron schedule (e.g. \"0 3 * * *\")")
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage backups",
	Long:  "Initialize, create, restore, and schedule borgmatic backups.",
}

var backupInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize borg repo and generate borgmatic configs",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		borg := backup.NewBorg(runner)

		// Initialize borg repository
		fmt.Println("Initializing borg repository...")
		if _, err := borg.Init(cfg.Backup.BorgRepo); err != nil {
			return fmt.Errorf("initializing borg repo: %w", err)
		}
		fmt.Println("Borg repository initialized.")

		// Generate borgmatic configs for all deployed apps
		registry, err := app.NewRegistry(templates.FS)
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}

		deployed, err := listDeployedApps(cfg)
		if err != nil {
			return err
		}

		for _, appName := range deployed {
			meta, ok := registry.Get(appName)
			if !ok {
				fmt.Fprintf(os.Stderr, "Warning: no template found for deployed app %s, skipping config generation\n", appName)
				continue
			}
			content := backup.GenerateConfig(appName, meta, cfg.Backup, cfg.DataDir)
			configDir := cfg.AppDir(appName)
			if err := backup.WriteConfig(appName, content, configDir); err != nil {
				return err
			}
			fmt.Printf("Generated borgmatic config for %s\n", appName)
		}

		return nil
	},
}

var backupCreateCmd = &cobra.Command{
	Use:   "create [app]",
	Short: "Create a backup",
	Long:  "Create a backup for a specific app or all deployed apps.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		borg := backup.NewBorg(runner)

		registry, err := app.NewRegistry(templates.FS)
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}

		var apps []string
		if len(args) == 1 {
			apps = []string{args[0]}
		} else {
			deployed, err := listDeployedApps(cfg)
			if err != nil {
				return err
			}
			// Only include apps that have backup config
			for _, name := range deployed {
				meta, ok := registry.Get(name)
				if ok && meta.Backup != nil {
					apps = append(apps, name)
				}
			}
		}

		if len(apps) == 0 {
			fmt.Println("No apps with backup configuration found.")
			return nil
		}

		for _, appName := range apps {
			configFile := backup.ConfigPath(cfg.AppsDir, appName)
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: no borgmatic config for %s, run 'backup init' first\n", appName)
				continue
			}

			meta, _ := registry.Get(appName)

			// Run pre-hook if defined
			if meta != nil && meta.Backup != nil && meta.Backup.PreHook != "" {
				fmt.Printf("[%s] Running pre-backup hook...\n", appName)
				if _, err := runner.Run("sh", "-c", meta.Backup.PreHook); err != nil {
					return fmt.Errorf("pre-backup hook for %s failed: %w", appName, err)
				}
			}

			fmt.Printf("[%s] Creating backup...\n", appName)
			if _, err := borg.Create(configFile); err != nil {
				return fmt.Errorf("backup failed for %s: %w", appName, err)
			}

			// Run post-hook if defined
			if meta != nil && meta.Backup != nil && meta.Backup.PostHook != "" {
				fmt.Printf("[%s] Running post-backup hook...\n", appName)
				if _, err := runner.Run("sh", "-c", meta.Backup.PostHook); err != nil {
					return fmt.Errorf("post-backup hook for %s failed: %w", appName, err)
				}
			}

			fmt.Printf("[%s] Backup complete.\n", appName)
		}

		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <app> [archive]",
	Short: "Restore an app from backup",
	Long:  "Stop the app, restore from an archive (latest if not specified), and restart.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		borg := backup.NewBorg(runner)
		mgr, err := newManager()
		if err != nil {
			return err
		}

		appName := args[0]
		var archive string
		if len(args) == 2 {
			archive = args[1]
		}

		configFile := backup.ConfigPath(cfg.AppsDir, appName)
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return fmt.Errorf("no borgmatic config for %s, run 'backup init' first", appName)
		}

		// Stop the app
		fmt.Printf("[%s] Stopping app...\n", appName)
		if err := mgr.Stop(appName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not stop %s: %v\n", appName, err)
		}

		// Restore
		fmt.Printf("[%s] Restoring from backup...\n", appName)
		if _, err := borg.Restore(configFile, archive); err != nil {
			return fmt.Errorf("restore failed for %s: %w", appName, err)
		}

		// Restart the app
		fmt.Printf("[%s] Starting app...\n", appName)
		if err := mgr.Start(appName); err != nil {
			return fmt.Errorf("failed to restart %s after restore: %w", appName, err)
		}

		fmt.Printf("[%s] Restore complete.\n", appName)
		return nil
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list [app]",
	Short: "List backup archives",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		borg := backup.NewBorg(runner)

		var apps []string
		if len(args) == 1 {
			apps = []string{args[0]}
		} else {
			deployed, err := listDeployedApps(cfg)
			if err != nil {
				return err
			}
			apps = deployed
		}

		for _, appName := range apps {
			configFile := backup.ConfigPath(cfg.AppsDir, appName)
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				continue
			}

			fmt.Printf("=== %s ===\n", appName)
			archives, err := borg.List(configFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error listing archives: %v\n", err)
				continue
			}

			if len(archives) == 0 {
				fmt.Println("  No archives found.")
				continue
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  ARCHIVE\tDATE")
			for _, a := range archives {
				fmt.Fprintf(w, "  %s\t%s\n", a.Name, a.Date)
			}
			w.Flush()
		}

		return nil
	},
}

var backupScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Show or set backup schedule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cronExpr, _ := cmd.Flags().GetString("cron")
		if cronExpr != "" {
			// Validate the cron expression by creating a scheduler
			sched := backup.NewScheduler()
			err := sched.Start(cronExpr, func() {})
			if err != nil {
				return fmt.Errorf("invalid cron expression: %w", err)
			}
			sched.Stop()

			fmt.Printf("Backup schedule set to: %s\n", cronExpr)
			fmt.Println("Note: update backup.schedule in your config file to persist this setting.")
			return nil
		}

		fmt.Printf("Current backup schedule: %s\n", cfg.Backup.Schedule)
		fmt.Printf("Backups enabled: %v\n", cfg.Backup.Enabled)
		fmt.Printf("Borg repository: %s\n", cfg.Backup.BorgRepo)
		return nil
	},
}

// listDeployedApps returns the names of all deployed apps by scanning the apps directory.
func listDeployedApps(cfg *config.Config) ([]string, error) {
	entries, err := os.ReadDir(cfg.AppsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading apps directory: %w", err)
	}

	var apps []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		infoPath := cfg.AppDir(entry.Name()) + "/.homelabctl.yaml"
		if _, err := os.Stat(infoPath); err == nil {
			apps = append(apps, entry.Name())
		}
	}
	return apps, nil
}

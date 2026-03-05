package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
)

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().Bool("dry-run", false, "Show what would change without applying")
	upgradeCmd.Flags().Bool("no-backup", false, "Skip pre-upgrade backup")
	upgradeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	upgradeCmd.ValidArgsFunction = completeDeployedApps
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <app>",
	Short: "Upgrade an app's template to the latest version",
	Long: `Upgrade a deployed app by re-rendering its template with the latest version.

Unlike 'update' (which only pulls new Docker images), 'upgrade' re-applies
the template, picking up any changes to docker-compose configuration,
security settings, or new features.

The upgrade flow:
  1. Back up the app (if backup is configured)
  2. Show a diff of what would change
  3. Re-render and apply the new template
  4. Recreate containers with the new config`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		appName := args[0]
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		noBackup, _ := cmd.Flags().GetBool("no-backup")
		yes, _ := cmd.Flags().GetBool("yes")

		// Verify the app is deployed
		info, err := mgr.GetDeployedInfo(appName)
		if err != nil {
			return fmt.Errorf("app %s is not deployed: %w", appName, err)
		}

		// Verify the template still exists
		meta, ok := mgr.Registry().Get(appName)
		if !ok {
			return fmt.Errorf("no template found for %s", appName)
		}

		// Compare versions
		fmt.Printf("App:              %s\n", appName)
		fmt.Printf("Deployed version: %s\n", info.Version)
		fmt.Printf("Template version: %s\n", meta.Version)

		if info.Version == meta.Version {
			fmt.Println("\nTemplate version unchanged. Re-rendering with current template.")
		} else {
			fmt.Printf("\nUpgrade available: %s -> %s\n", info.Version, meta.Version)
		}

		// Re-render templates with the stored values
		renderer := app.NewTemplateRenderer(mgr.Registry())
		rendered, err := renderer.RenderAllFiles(appName, info.Values)
		if err != nil {
			return fmt.Errorf("rendering templates: %w", err)
		}

		// Show diff of what would change
		appDir := cfg.AppDir(appName)
		fmt.Println("\nChanges:")
		hasChanges := false
		for name, newContent := range rendered {
			existingPath := filepath.Join(appDir, name)
			existingData, err := os.ReadFile(existingPath)
			if err != nil {
				fmt.Printf("  + %s (new file)\n", name)
				hasChanges = true
				continue
			}
			if string(existingData) != newContent {
				fmt.Printf("  ~ %s (modified)\n", name)
				hasChanges = true
			}
		}

		if !hasChanges {
			fmt.Println("  No changes detected.")
			if dryRun {
				return nil
			}
		}

		if dryRun {
			fmt.Println("\nDry run — no changes applied.")
			return nil
		}

		if !yes && !app.AskConfirmation("Apply upgrade?") {
			return fmt.Errorf("upgrade cancelled by user")
		}

		// Pre-upgrade backup
		if !noBackup && cfg.Backup.Enabled {
			configFile := backup.ConfigPath(cfg.AppsDir, appName)
			if _, statErr := os.Stat(configFile); statErr == nil {
				fmt.Printf("Backing up %s before upgrade...\n", appName)
				runner := &exec.Runner{Verbose: verbose}
				borg := backup.NewBorg(runner)
				if _, borgErr := borg.Create(configFile); borgErr != nil {
					return fmt.Errorf("pre-upgrade backup failed: %w (use --no-backup to skip)", borgErr)
				}
			}
		}

		// Apply rendered files
		for name, content := range rendered {
			outPath := filepath.Join(appDir, name)
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", name, err)
			}
			if err := os.WriteFile(outPath, []byte(content), 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", name, err)
			}
		}

		// Update the deployment info with new version
		info.Version = meta.Version
		if err := mgr.SaveDeployedInfo(appName, info); err != nil {
			return fmt.Errorf("updating deploy info: %w", err)
		}

		// Recreate containers
		runner := &exec.Runner{Verbose: verbose}
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
		fmt.Printf("Recreating containers for %s...\n", appName)
		if _, err := compose.Up(appDir); err != nil {
			return fmt.Errorf("recreating containers: %w", err)
		}

		fmt.Printf("\nApp %s upgraded to version %s.\n", appName, meta.Version)

		// Show access info
		hostname := info.Values["hostname"]
		domain := info.Values["domain"]
		fqdn := hostname + "." + domain
		for _, p := range meta.Ports {
			if p.ValueName != "" {
				if portVal, ok := info.Values[p.ValueName]; ok {
					scheme := "http"
					if strings.Contains(strings.ToLower(p.Description), "https") {
						scheme = "https"
					}
					fmt.Printf("  %-20s %s://%s:%s\n", p.Description+":", scheme, fqdn, portVal)
				}
			}
		}
		return nil
	},
}

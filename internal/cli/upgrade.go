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
	upgradeCmd.Flags().Bool("check", false, "Only show available image updates, don't apply")
	upgradeCmd.Flags().Bool("patch-only", false, "Only apply patch-level image updates")
	upgradeCmd.Flags().Bool("all", false, "Upgrade all deployed apps")
	upgradeCmd.ValidArgsFunction = completeDeployedApps
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [app]",
	Short: "Upgrade an app's template or container images",
	Long: `Upgrade a deployed app by re-rendering its template with the latest version,
or by upgrading container images to newer semver versions.

When container images have pinned semver tags (e.g., nextcloud:32.0.6), this
command can detect and apply newer versions from the registry.

The upgrade flow:
  1. Check for newer container image versions
  2. Back up the app (if backup is configured)
  3. Show a diff of what would change
  4. Re-render and apply the new template (if template changed)
  5. Update image tags in docker-compose.yml (if newer images found)
  6. Recreate containers with the new config

Flags:
  --check       Only show available image updates, don't apply changes
  --patch-only  Only apply patch-level image updates (e.g., 32.0.6 -> 32.0.9)
  --all         Upgrade all deployed apps`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		noBackup, _ := cmd.Flags().GetBool("no-backup")
		yes, _ := cmd.Flags().GetBool("yes")
		check, _ := cmd.Flags().GetBool("check")
		patchOnly, _ := cmd.Flags().GetBool("patch-only")
		all, _ := cmd.Flags().GetBool("all")

		if all {
			deployed, err := mgr.ListDeployed()
			if err != nil {
				return err
			}
			if len(deployed) == 0 {
				fmt.Println("No apps deployed.")
				return nil
			}
			var errs []string
			for _, appName := range deployed {
				fmt.Printf("=== %s ===\n", appName)
				if err := runUpgrade(cfg, mgr, appName, dryRun, noBackup, yes, check, patchOnly); err != nil {
					fmt.Printf("  Error: %v\n", err)
					errs = append(errs, appName)
				}
				fmt.Println()
			}
			if len(errs) > 0 {
				return fmt.Errorf("upgrade failed for: %s", strings.Join(errs, ", "))
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("app name required (or use --all)")
		}
		return runUpgrade(cfg, mgr, args[0], dryRun, noBackup, yes, check, patchOnly)
	},
}

func runUpgrade(cfg *config.Config, mgr *app.Manager, appName string, dryRun, noBackup, yes, check, patchOnly bool) error {
	// Verify the app is deployed
	info, err := mgr.GetDeployedInfo(appName)
	if err != nil {
		return fmt.Errorf("app %s is not deployed: %w", appName, err)
	}

	appDir := cfg.AppDir(appName)

	// --- Image version check ---
	composePath := filepath.Join(appDir, "docker-compose.yml")
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("reading compose file: %w", err)
	}

	refs, _ := app.ScanDeployedImages(composeData)
	var imageUpdates []imageUpdatePlan
	resolver := app.NewImageResolver()

	for _, ref := range refs {
		if _, err := app.ParseSemver(ref.Tag); err != nil {
			continue // skip non-semver images
		}
		updates, err := resolver.FindNewerVersions(ref)
		if err != nil {
			fmt.Printf("  Warning: could not check %s: %v\n", ref.String(), err)
			continue
		}
		for _, u := range updates {
			if patchOnly && u.Type != "patch" {
				continue
			}
			imageUpdates = append(imageUpdates, imageUpdatePlan{
				ref:    ref,
				update: u,
			})
		}
	}

	if check {
		// --check mode: just display available updates
		if len(imageUpdates) == 0 {
			fmt.Printf("No image updates available for %s.\n", appName)
			return nil
		}
		fmt.Printf("Available image updates for %s:\n", appName)
		for _, iu := range imageUpdates {
			fmt.Printf("  %s: %s -> %s (%s)\n", iu.ref.String(), iu.update.CurrentTag, iu.update.NewTag, iu.update.Type)
		}
		return nil
	}

	// --- Template upgrade check ---
	meta, ok := mgr.Registry().Get(appName)
	templateChanged := false
	if ok {
		fmt.Printf("App:              %s\n", appName)
		fmt.Printf("Deployed version: %s\n", info.Version)
		fmt.Printf("Template version: %s\n", meta.Version)

		if info.Version != meta.Version {
			fmt.Printf("\nTemplate upgrade available: %s -> %s\n", info.Version, meta.Version)
			templateChanged = true
		}
	}

	// Show image updates
	if len(imageUpdates) > 0 {
		fmt.Println("\nImage updates:")
		for _, iu := range imageUpdates {
			fmt.Printf("  %s: %s -> %s (%s)\n", iu.ref.String(), iu.update.CurrentTag, iu.update.NewTag, iu.update.Type)
		}
	}

	// Check if there's anything to do
	hasTemplateChanges := false
	var rendered map[string]string
	if ok && templateChanged {
		renderer := app.NewTemplateRenderer(mgr.Registry())
		rendered, err = renderer.RenderAllFiles(appName, info.Values)
		if err != nil {
			return fmt.Errorf("rendering templates: %w", err)
		}

		fmt.Println("\nTemplate changes:")
		for name, newContent := range rendered {
			existingPath := filepath.Join(appDir, name)
			existingData, err := os.ReadFile(existingPath)
			if err != nil {
				fmt.Printf("  + %s (new file)\n", name)
				hasTemplateChanges = true
				continue
			}
			if string(existingData) != newContent {
				fmt.Printf("  ~ %s (modified)\n", name)
				hasTemplateChanges = true
			}
		}
		if !hasTemplateChanges {
			fmt.Println("  No template file changes detected.")
		}
	}

	if !hasTemplateChanges && len(imageUpdates) == 0 {
		fmt.Println("\nNothing to upgrade.")
		return nil
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

	// Save original compose file for rollback on failure
	origCompose, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("reading compose file for backup: %w", err)
	}

	// Apply template changes
	if hasTemplateChanges && rendered != nil {
		for name, content := range rendered {
			outPath := filepath.Join(appDir, name)
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", name, err)
			}
			if err := os.WriteFile(outPath, []byte(content), 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", name, err)
			}
		}
	}

	// Apply image tag updates to the deployed docker-compose.yml
	if len(imageUpdates) > 0 {
		content, err := os.ReadFile(composePath)
		if err != nil {
			return fmt.Errorf("reading compose file for image update: %w", err)
		}
		text := string(content)
		for _, iu := range imageUpdates {
			oldImage := iu.ref.String()
			newRef := iu.ref
			newRef.Tag = iu.update.NewTag
			newImage := newRef.String()
			text = strings.ReplaceAll(text, oldImage, newImage)
		}
		if err := os.WriteFile(composePath, []byte(text), 0o600); err != nil {
			return fmt.Errorf("writing updated compose file: %w", err)
		}
	}

	// Recreate containers — rollback compose file on failure
	runner := &exec.Runner{Verbose: verbose}
	compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
	fmt.Printf("Recreating containers for %s...\n", appName)
	if _, err := compose.Up(appDir); err != nil {
		fmt.Printf("Container recreation failed, rolling back compose file...\n")
		if rollbackErr := os.WriteFile(composePath, origCompose, 0o600); rollbackErr != nil {
			return fmt.Errorf("recreating containers: %w (additionally, rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("recreating containers (rolled back): %w", err)
	}

	// Update deployment info only after successful container recreation
	if ok && templateChanged {
		info.Version = meta.Version
	}
	if err := mgr.SaveDeployedInfo(appName, info); err != nil {
		return fmt.Errorf("updating deploy info: %w", err)
	}

	fmt.Printf("\nApp %s upgraded successfully.\n", appName)

	// Show access info
	if ok {
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
	}
	return nil
}

// imageUpdatePlan pairs an image ref with its available update.
type imageUpdatePlan struct {
	ref    app.ImageRef
	update app.VersionUpdate
}

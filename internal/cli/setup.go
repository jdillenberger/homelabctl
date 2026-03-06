package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/wizard"
	"github.com/jdillenberger/homelabctl/templates"
)

func init() {
	setupCmd.Flags().StringP("config-path", "p", "/etc/homelabctl/config.yaml", "Path to write the config file")
	setupCmd.Flags().Bool("deploy", true, "Deploy selected apps after setup")
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the interactive setup wizard",
	Long:  "Run a guided setup wizard to configure homelabctl, select apps, and optionally deploy them.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, _ := cmd.Flags().GetString("config-path")
		deploy, _ := cmd.Flags().GetBool("deploy")

		cfg := config.DefaultConfig()

		// Load available app template names from the embedded registry.
		mgr, err := newManager()
		if err != nil {
			return fmt.Errorf("loading app templates: %w", err)
		}
		availableApps := mgr.Registry().List()

		// Run the wizard
		selectedApps, err := wizard.RunSetupWizard(cfg, availableApps)
		if err != nil {
			return err
		}

		// Write config file
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling config: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			return fmt.Errorf("writing config file: %w", err)
		}
		fmt.Printf("Configuration written to %s\n", cfgPath)

		// Reload viper with the new config so subsequent operations use it.
		viper.SetConfigFile(cfgPath)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("re-reading config: %w", err)
		}

		// Ensure directories exist
		if err := cfg.EnsureDirectories(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create directories: %v\n", err)
		}

		// Prepend traefik to selected apps when ingress is enabled
		if cfg.Ingress.Enabled && cfg.Ingress.Provider == "traefik" {
			hasTraefik := false
			for _, a := range selectedApps {
				if a == "traefik" {
					hasTraefik = true
					break
				}
			}
			if !hasTraefik {
				selectedApps = append([]string{"traefik"}, selectedApps...)
			}
		}

		// Deploy selected apps
		if deploy && len(selectedApps) > 0 {
			fmt.Println("\nDeploying selected apps...")
			runner := &exec.Runner{Verbose: verbose}
			deployMgr, err := app.NewManager(cfg, runner, templates.FS)
			if err != nil {
				return fmt.Errorf("creating app manager: %w", err)
			}

			for _, appName := range selectedApps {
				fmt.Printf("\n--- Deploying %s ---\n", appName)
				meta, ok := deployMgr.Registry().Get(appName)
				if !ok {
					fmt.Fprintf(os.Stderr, "Warning: unknown app template %q, skipping.\n", appName)
					continue
				}

				values, err := wizard.RunDeployWizard(meta)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", appName, err)
					continue
				}

				if err := deployMgr.Deploy(appName, app.DeployOptions{
					Values:  values,
					DryRun:  false,
					Confirm: true, // skip confirmation in setup wizard (already confirmed)
				}); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to deploy %s: %v\n", appName, err)
				}
			}
		}

		fmt.Println("\nSetup complete.")
		return nil
	},
}

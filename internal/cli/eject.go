package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
)

func init() {
	rootCmd.AddCommand(ejectCmd)
	ejectCmd.Flags().StringP("output", "o", "", "Output directory (default: ./homelabctl-eject)")
}

var ejectCmd = &cobra.Command{
	Use:   "eject",
	Short: "Export all generated configs for standalone use",
	Long: `Export all deployed app configs (docker-compose.yml, .env, borgmatic.yaml)
to a directory, so you can manage them without homelabctl.

This is the ultimate transparency feature: everything homelabctl generates
is standard docker-compose and borgmatic configuration. You can walk away
from homelabctl at any time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		outputDir, _ := cmd.Flags().GetString("output")
		if outputDir == "" {
			outputDir = "homelabctl-eject"
		}

		deployed, err := mgr.ListDeployed()
		if err != nil {
			return err
		}

		if len(deployed) == 0 {
			fmt.Println("No apps deployed. Nothing to eject.")
			return nil
		}

		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Files to copy from each app directory.
		filesToCopy := []string{
			"docker-compose.yml",
			".env",
			"borgmatic.yaml",
		}

		var ejected int
		for _, appName := range deployed {
			appDir := cfg.AppDir(appName)
			destDir := filepath.Join(outputDir, appName)
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				fmt.Printf("  %s: failed to create directory: %v\n", appName, err)
				continue
			}

			copied := 0
			for _, f := range filesToCopy {
				srcPath := filepath.Join(appDir, f)
				data, err := os.ReadFile(srcPath)
				if err != nil {
					continue // file doesn't exist for this app
				}
				destPath := filepath.Join(destDir, f)
				if err := os.WriteFile(destPath, data, 0o644); err != nil {
					fmt.Printf("  %s: failed to write %s: %v\n", appName, f, err)
					continue
				}
				copied++
			}

			if copied > 0 {
				fmt.Printf("  %s: exported %d file(s)\n", appName, copied)
				ejected++
			}
		}

		fmt.Printf("\nEjected %d app(s) to %s/\n", ejected, outputDir)
		fmt.Println("\nYou can now manage these apps with standard tools:")
		fmt.Println("  cd <app-dir> && docker compose up -d")
		fmt.Println("  borgmatic -c borgmatic.yaml create")
		return nil
	},
}

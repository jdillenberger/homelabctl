package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/export"
)

func init() {
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().Bool("dry-run", false, "Validate without deploying")
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export homelab configuration and deployed apps",
	Long:  "Export the full homelab configuration and deployed apps as YAML to stdout.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		exp, err := export.BuildExport(cfg, mgr)
		if err != nil {
			return err
		}

		data, err := yaml.Marshal(exp)
		if err != nil {
			return fmt.Errorf("marshaling export: %w", err)
		}

		fmt.Fprint(os.Stdout, string(data))
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import homelab configuration from an export file",
	Long:  "Import and deploy apps from a previously exported YAML file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		exp, err := export.ParseExportFile(args[0])
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		fmt.Printf("Importing from %s (version: %s)\n", args[0], exp.Version)
		if dryRun {
			fmt.Println("Dry run mode — no changes will be made.")
		}
		fmt.Printf("Apps to process: %d\n\n", len(exp.Apps))

		return export.Import(exp, mgr, dryRun)
	},
}

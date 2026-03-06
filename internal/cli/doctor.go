package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/doctor"
)

func init() {
	doctorCmd.Flags().Bool("fix", false, "Auto-install missing dependencies")
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system dependencies",
	Long:  "Verify that all required tools are installed and report their versions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fix, _ := cmd.Flags().GetBool("fix")

		results := doctor.CheckAll()

		if jsonOutput {
			return outputJSON(results)
		}

		allOK := true
		for _, r := range results {
			if r.Installed {
				fmt.Printf("  [x] %-20s %s\n", r.Name, r.Version)
			} else {
				if r.Version != "" {
				fmt.Printf("  [ ] %-20s %s\n", r.Name, r.Version)
			} else if r.InstallCommand != "" {
				fmt.Printf("  [ ] %-20s missing (install: %s)\n", r.Name, r.InstallCommand)
			} else {
				fmt.Printf("  [ ] %-20s missing (run --fix to configure)\n", r.Name)
			}
				allOK = false
			}
		}

		if allOK {
			fmt.Println("\nAll dependencies are installed.")
			return nil
		}

		if !fix {
			fmt.Println("\nSome dependencies are missing. Run with --fix to install them.")
			return nil
		}

		fmt.Println("\nInstalling missing dependencies...")
		for _, r := range results {
			if r.Installed {
				continue
			}
			fmt.Printf("  Installing %s...\n", r.Name)
			if err := doctor.Fix(r); err != nil {
				fmt.Printf("    Failed: %v\n", err)
			} else {
				fmt.Printf("    Installed %s.\n", r.Name)
			}
		}

		return nil
	},
}

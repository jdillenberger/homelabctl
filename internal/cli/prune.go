package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/exec"
)

func init() {
	pruneCmd.Flags().Bool("force", false, "Actually prune (default is dry-run)")
	rootCmd.AddCommand(pruneCmd)
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up unused Docker resources",
	Long:  "Remove dangling images, unused volumes, networks, and build cache. Without --force, shows a dry-run.",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		runner := &exec.Runner{Verbose: verbose}

		if !force {
			fmt.Println("Dry run — showing resources that would be cleaned up:")

			// Dangling images
			result, err := runner.Run("docker", "image", "ls", "--filter", "dangling=true", "--format", "{{.Repository}}:{{.Tag}}\t{{.Size}}")
			if err == nil && strings.TrimSpace(result.Stdout) != "" {
				fmt.Println("Dangling images:")
				fmt.Println(result.Stdout)
			} else {
				fmt.Println("No dangling images.")
			}

			// Unused volumes
			result, err = runner.Run("docker", "volume", "ls", "--filter", "dangling=true", "--format", "{{.Name}}")
			if err == nil && strings.TrimSpace(result.Stdout) != "" {
				fmt.Println("Unused volumes:")
				fmt.Println(result.Stdout)
			} else {
				fmt.Println("No unused volumes.")
			}

			fmt.Println("\nRun with --force to remove these resources.")
			return nil
		}

		type pruneOp struct {
			label string
			args  []string
		}

		ops := []pruneOp{
			{"images", []string{"image", "prune", "-af"}},
			{"volumes", []string{"volume", "prune", "-f"}},
			{"networks", []string{"network", "prune", "-f"}},
			{"build cache", []string{"builder", "prune", "-af"}},
		}

		for _, op := range ops {
			fmt.Printf("Pruning %s...\n", op.label)
			result, err := runner.Run("docker", op.args...)
			if err != nil {
				fmt.Printf("  Warning: %v\n", err)
				continue
			}
			// Parse "Total reclaimed space" line if present
			for _, line := range strings.Split(result.Stdout, "\n") {
				if strings.Contains(line, "reclaimed") || strings.Contains(line, "deleted") || strings.Contains(line, "Deleted") {
					fmt.Printf("  %s\n", strings.TrimSpace(line))
				}
			}
		}

		fmt.Println("Prune complete.")
		return nil
	},
}

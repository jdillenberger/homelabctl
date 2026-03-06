package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/repo"
)

func init() {
	templatesCmd.AddCommand(templatesRepoCmd)
	templatesRepoCmd.AddCommand(templatesRepoAddCmd)
	templatesRepoCmd.AddCommand(templatesRepoRemoveCmd)
	templatesRepoCmd.AddCommand(templatesRepoListCmd)
	templatesRepoCmd.AddCommand(templatesRepoUpdateCmd)

	templatesRepoAddCmd.Flags().String("name", "", "Override the repo name (default: derived from URL)")
	templatesRepoAddCmd.Flags().String("ref", "", "Branch or tag to clone")
	templatesRepoRemoveCmd.ValidArgsFunction = completeRepoNames
	templatesRepoUpdateCmd.ValidArgsFunction = completeRepoNames
}

func newRepoManager() (*repo.Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	runner := &exec.Runner{Verbose: verbose}
	return repo.NewManager(cfg.ReposDir(), cfg.ManifestPath(), runner), nil
}

// completeRepoNames returns repo names for shell completion.
func completeRepoNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	mgr, err := newRepoManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	repos, err := mgr.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var templatesRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage git repository template sources",
	Long:  "Add, remove, list, and update git repositories used as template sources.",
}

var templatesRepoAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a git repository as a template source",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newRepoManager()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		ref, _ := cmd.Flags().GetString("ref")

		r, err := mgr.Add(args[0], name, ref)
		if err != nil {
			return err
		}

		fmt.Printf("Added repo %q from %s\n", r.Name, r.URL)
		return nil
	},
}

var templatesRepoRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a template repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newRepoManager()
		if err != nil {
			return err
		}

		if err := mgr.Remove(args[0]); err != nil {
			return err
		}

		fmt.Printf("Removed repo %q\n", args[0])
		return nil
	},
}

var templatesRepoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List template repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newRepoManager()
		if err != nil {
			return err
		}

		repos, err := mgr.List()
		if err != nil {
			return err
		}

		if len(repos) == 0 {
			fmt.Println("No template repositories configured.")
			return nil
		}

		if jsonOutput {
			return outputJSON(repos)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tURL\tREF\tADDED")
		for _, r := range repos {
			ref := r.Ref
			if ref == "" {
				ref = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.URL, ref, r.AddedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

var templatesRepoUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update template repositories",
	Long:  "Pull the latest changes for a specific repo or all repos.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newRepoManager()
		if err != nil {
			return err
		}

		if len(args) > 0 {
			if err := mgr.Update(args[0]); err != nil {
				return err
			}
			fmt.Printf("Updated repo %q\n", args[0])
			return nil
		}

		repos, err := mgr.List()
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			fmt.Println("No template repositories configured.")
			return nil
		}
		for _, r := range repos {
			fmt.Printf("Updating %s...\n", r.Name)
			if err := mgr.Update(r.Name); err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			fmt.Printf("  %s updated.\n", r.Name)
		}
		return nil
	},
}

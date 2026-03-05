package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/templates"
)

func init() {
	rootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesExportCmd)
	templatesCmd.AddCommand(templatesDeleteCmd)
	templatesCmd.AddCommand(templatesPathCmd)

	templatesExportCmd.Flags().Bool("force", false, "Overwrite existing local template")
	templatesExportCmd.ValidArgsFunction = completeTemplateNames
	templatesDeleteCmd.ValidArgsFunction = completeLocalTemplates
}

// completeLocalTemplates returns local template override names for shell completion.
func completeLocalTemplates(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	entries, err := os.ReadDir(cfg.TemplatesDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage app templates",
	Long:  "List, export, and manage app templates. Local templates in ~/.homelabctl/templates/ override built-in ones.",
}

var templatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates with source info",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		type templateEntry struct {
			Name        string `json:"name"`
			Category    string `json:"category"`
			Description string `json:"description"`
			Source      string `json:"source"`
		}

		var entries []templateEntry
		for _, meta := range mgr.Registry().All() {
			source := "built-in"
			if overlay, ok := mgr.Registry().FS().(*app.OverlayFS); ok {
				source = overlay.Source(meta.Name)
			}
			entries = append(entries, templateEntry{
				Name:        meta.Name,
				Category:    meta.Category,
				Description: meta.Description,
				Source:      source,
			})
		}

		if jsonOutput {
			return outputJSON(entries)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCATEGORY\tSOURCE\tDESCRIPTION")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.Category, e.Source, e.Description)
		}
		w.Flush()
		return nil
	},
}

var templatesExportCmd = &cobra.Command{
	Use:   "export <template>",
	Short: "Export a built-in template to the local templates directory for customization",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		templateName := args[0]
		force, _ := cmd.Flags().GetBool("force")

		// Verify the template exists in the embedded FS.
		registry, err := app.NewRegistry(templates.FS)
		if err != nil {
			return err
		}
		if _, ok := registry.Get(templateName); !ok {
			return fmt.Errorf("unknown built-in template: %s", templateName)
		}

		destDir := filepath.Join(cfg.TemplatesDir, templateName)
		if _, err := os.Stat(destDir); err == nil && !force {
			return fmt.Errorf("local template %s already exists at %s (use --force to overwrite)", templateName, destDir)
		}

		// Walk the embedded template directory and copy all files.
		tmplDir := templateName
		err = fs.WalkDir(templates.FS, tmplDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			relPath, _ := filepath.Rel(tmplDir, path)
			destPath := filepath.Join(destDir, relPath)

			if d.IsDir() {
				return os.MkdirAll(destPath, 0o755)
			}

			data, err := fs.ReadFile(templates.FS, path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}

			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			return os.WriteFile(destPath, data, 0o644)
		})
		if err != nil {
			return fmt.Errorf("exporting template: %w", err)
		}

		fmt.Printf("Exported %s to %s\n", templateName, destDir)
		fmt.Println("Edit the files there to customize. Your local version will override the built-in template.")
		return nil
	},
}

var templatesDeleteCmd = &cobra.Command{
	Use:   "delete <template>",
	Short: "Remove a local template override",
	Long:  "Delete a local template from ~/.homelabctl/templates/, restoring the built-in version.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		templateName := args[0]
		localDir := filepath.Join(cfg.TemplatesDir, templateName)

		if _, err := os.Stat(localDir); os.IsNotExist(err) {
			return fmt.Errorf("no local template %q found in %s", templateName, cfg.TemplatesDir)
		}

		if err := os.RemoveAll(localDir); err != nil {
			return fmt.Errorf("removing local template: %w", err)
		}

		// Check if a built-in version exists.
		registry, err := app.NewRegistry(templates.FS)
		if err == nil {
			if _, ok := registry.Get(templateName); ok {
				fmt.Printf("Removed local template %s (built-in version restored).\n", templateName)
				return nil
			}
		}

		fmt.Printf("Removed local template %s.\n", templateName)
		return nil
	},
}

var templatesPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the local templates directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Println(cfg.TemplatesDir)
		return nil
	},
}

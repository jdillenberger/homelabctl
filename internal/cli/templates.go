package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/repo"
	"github.com/jdillenberger/homelabctl/templates"
)

func init() {
	rootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesExportCmd)
	templatesCmd.AddCommand(templatesDeleteCmd)
	templatesCmd.AddCommand(templatesPathCmd)
	templatesCmd.AddCommand(templatesNewCmd)
	templatesCmd.AddCommand(templatesLintCmd)

	templatesLintCmd.Flags().Bool("suppress", false, "Add all current warnings/infos to lint_ignore in app.yaml")
	templatesLintCmd.ValidArgsFunction = completeTemplateNames
	templatesExportCmd.Flags().Bool("force", false, "Overwrite existing local template")
	templatesExportCmd.ValidArgsFunction = completeTemplateNames
	templatesDeleteCmd.ValidArgsFunction = completeLocalTemplates
	templatesNewCmd.Flags().Bool("dockerfile", false, "Generate Dockerfile-based template (uses build: instead of image:)")
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

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runner := &exec.Runner{Verbose: verbose}
		repoMgr := repo.NewManager(cfg.ReposDir(), cfg.ManifestPath(), runner)
		repoNames, _ := repoMgr.RepoNames()

		var entries []templateEntry
		for _, meta := range mgr.Registry().All() {
			source := app.ResolveSource(mgr.Registry().FS(), meta.Name, repoNames)
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

var templatesNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a new app template",
	Long:  "Create a skeleton template in the local templates directory with app.yaml, docker-compose.yml.tmpl, and .env.tmpl.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		name := args[0]
		destDir := filepath.Join(cfg.TemplatesDir, name)
		dockerfile, _ := cmd.Flags().GetBool("dockerfile")

		if _, err := os.Stat(destDir); err == nil {
			return fmt.Errorf("template %q already exists at %s", name, destDir)
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return fmt.Errorf("creating template directory: %w", err)
		}

		requiresBuildLine := ""
		if dockerfile {
			requiresBuildLine = "\nrequires_build: true\n"
		}

		appYAML := fmt.Sprintf(`name: %s
description: "TODO: Add description"
category: "custom"
version: "1.0.0"
%s
ports:
  - host: 8080
    container: 8080
    protocol: tcp
    description: "Web UI"
    value_name: web_port

volumes:
  - name: data
    container: /data
    description: "Application data"

values:
  - name: web_port
    description: "Web UI port"
    default: "8080"
    required: true

health_check:
  url: "http://localhost:{{.web_port}}"
  interval: "30s"

# backup:
#   paths: []
#   pre_hook: ""
#   post_hook: ""

# post_deploy_info:
#   access_url: "http://{{.hostname}}.{{.domain}}:{{.web_port}}"
#   credentials: "See the app documentation"
#   notes:
#     - "Complete the initial setup wizard"

# hooks:
#   post_deploy:
#     - type: exec
#       command: "echo 'Deployed {{.app_name}}'"
`, name, requiresBuildLine)

		var composeTmpl string
		if dockerfile {
			composeTmpl = fmt.Sprintf(`services:
  %s:
    build: .
    container_name: %s
    restart: unless-stopped
    ports:
      - "{{.web_port}}:8080"
    volumes:
      - {{.data_dir}}/data:/data
    networks:
      - {{.network}}
    security_opt:
      - no-new-privileges:true
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    pids_limit: 256
    # mem_limit: 512m
    # cpus: 1.0

networks:
  {{.network}}:
    external: true
`, name, name)
		} else {
			composeTmpl = fmt.Sprintf(`services:
  %s:
    image: TODO_IMAGE:latest
    container_name: %s
    restart: unless-stopped
    ports:
      - "{{.web_port}}:8080"
    volumes:
      - {{.data_dir}}/data:/data
    networks:
      - {{.network}}
    security_opt:
      - no-new-privileges:true
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    pids_limit: 256
    # mem_limit: 512m
    # cpus: 1.0

networks:
  {{.network}}:
    external: true
`, name, name)
		}

		envTmpl := `# Environment variables for {{.app_name}}
TZ={{.timezone}}
`

		files := map[string]string{
			"app.yaml":                appYAML,
			"docker-compose.yml.tmpl": composeTmpl,
			".env.tmpl":               envTmpl,
		}

		if dockerfile {
			files["Dockerfile"] = `FROM alpine:3.21

# Install dependencies
# RUN apk add --no-cache <packages>

WORKDIR /app

# Copy application files
# COPY . .

EXPOSE 8080

CMD ["echo", "TODO: replace with your app command"]
`
			files[".dockerignore"] = `.git
.gitignore
*.md
.env
`
		}

		for fname, content := range files {
			fpath := filepath.Join(destDir, fname)
			if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", fname, err)
			}
		}

		fmt.Printf("Created template scaffold at %s/\n", destDir)
		fmt.Println("\nFiles created:")
		fmt.Println("  app.yaml                  - Template metadata (edit this first)")
		fmt.Println("  docker-compose.yml.tmpl   - Docker Compose template")
		fmt.Println("  .env.tmpl                 - Environment variables template")
		if dockerfile {
			fmt.Println("  Dockerfile                - Container build instructions")
			fmt.Println("  .dockerignore             - Files excluded from build context")
		}
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Edit %s/app.yaml with your app's metadata\n", destDir)
		if dockerfile {
			fmt.Printf("  2. Edit %s/Dockerfile with your build instructions\n", destDir)
			fmt.Printf("  3. Edit %s/docker-compose.yml.tmpl with your app's compose config\n", destDir)
			fmt.Printf("  4. Deploy with: homelabctl deploy %s\n", name)
		} else {
			fmt.Printf("  2. Edit %s/docker-compose.yml.tmpl with your app's compose config\n", destDir)
			fmt.Printf("  3. Deploy with: homelabctl deploy %s\n", name)
		}
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

var templatesLintCmd = &cobra.Command{
	Use:   "lint [template]",
	Short: "Check templates for common issues and best practices",
	Long:  "Lint app templates for missing restart policies, security hardening, health checks, and other best practices.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		suppress, _ := cmd.Flags().GetBool("suppress")

		if suppress && len(args) == 0 {
			return fmt.Errorf("--suppress requires a template name")
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		linter := app.NewLinter(mgr.Registry())

		if len(args) == 1 {
			findings := linter.LintTemplate(args[0])

			if suppress {
				return suppressFindings(args[0], findings)
			}

			result := &app.LintResult{Findings: findings}
			result.Summary = app.CountSummary(findings)

			if jsonOutput {
				return outputJSON(result)
			}

			printLintFindings(args[0], findings)
			fmt.Printf("\nSummary: %d error(s), %d warning(s), %d info(s) (%d total)\n",
				result.Summary.Errors, result.Summary.Warnings, result.Summary.Infos, result.Summary.Total)

			if result.Summary.Errors > 0 {
				return fmt.Errorf("lint found %d error(s)", result.Summary.Errors)
			}
			return nil
		}

		// Lint all templates
		result := linter.LintAll()

		if jsonOutput {
			return outputJSON(result)
		}

		// Group findings by template
		byTemplate := make(map[string][]app.LintFinding)
		for _, f := range result.Findings {
			byTemplate[f.Template] = append(byTemplate[f.Template], f)
		}

		for _, name := range mgr.Registry().List() {
			findings, ok := byTemplate[name]
			if !ok {
				continue
			}
			printLintFindings(name, findings)
			fmt.Println()
		}

		fmt.Printf("Summary: %d error(s), %d warning(s), %d info(s) (%d total)\n",
			result.Summary.Errors, result.Summary.Warnings, result.Summary.Infos, result.Summary.Total)

		if result.Summary.Errors > 0 {
			return fmt.Errorf("lint found %d error(s)", result.Summary.Errors)
		}
		return nil
	},
}

func printLintFindings(name string, findings []app.LintFinding) {
	fmt.Printf("=== %s ===\n", name)
	for _, f := range findings {
		var severity string
		switch f.Severity {
		case app.SeverityError:
			severity = "E"
		case app.SeverityWarning:
			severity = "W"
		case app.SeverityInfo:
			severity = "I"
		}
		fmt.Printf("  [%s] %s: %s\n", severity, f.File, f.Message)
	}
}

// suppressFindings collects non-ERROR findings and appends them to lint_ignore
// in the template's app.yaml. For built-in templates, exports them first.
func suppressFindings(templateName string, findings []app.LintFinding) error {
	// Collect unique check IDs from non-ERROR findings
	seen := make(map[string]bool)
	var checks []string
	for _, f := range findings {
		if f.Severity == app.SeverityError {
			continue
		}
		if !seen[f.Check] {
			seen[f.Check] = true
			checks = append(checks, f.Check)
		}
	}

	if len(checks) == 0 {
		fmt.Println("Nothing to suppress.")
		return nil
	}

	sort.Strings(checks)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Resolve the writable app.yaml path
	localDir := filepath.Join(cfg.TemplatesDir, templateName)
	appYAMLPath := filepath.Join(localDir, "app.yaml")

	if _, err := os.Stat(appYAMLPath); os.IsNotExist(err) {
		// Not a local template — export it first
		registry, regErr := app.NewRegistry(templates.FS)
		if regErr != nil {
			return regErr
		}
		if _, ok := registry.Get(templateName); !ok {
			return fmt.Errorf("unknown built-in template: %s", templateName)
		}

		fmt.Printf("Exporting %s to %s for local customization...\n", templateName, localDir)
		tmplDir := templateName
		exportErr := fs.WalkDir(templates.FS, tmplDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, _ := filepath.Rel(tmplDir, path)
			destPath := filepath.Join(localDir, relPath)
			if d.IsDir() {
				return os.MkdirAll(destPath, 0o755)
			}
			data, readErr := fs.ReadFile(templates.FS, path)
			if readErr != nil {
				return readErr
			}
			if mkdirErr := os.MkdirAll(filepath.Dir(destPath), 0o755); mkdirErr != nil {
				return mkdirErr
			}
			return os.WriteFile(destPath, data, 0o644)
		})
		if exportErr != nil {
			return fmt.Errorf("exporting template: %w", exportErr)
		}
	}

	// Read existing app.yaml
	data, err := os.ReadFile(appYAMLPath)
	if err != nil {
		return fmt.Errorf("reading app.yaml: %w", err)
	}

	content := string(data)

	// Determine which checks are already suppressed
	var newChecks []string
	for _, check := range checks {
		if !strings.Contains(content, check) {
			newChecks = append(newChecks, check)
		}
	}

	if len(newChecks) == 0 {
		fmt.Println("All findings are already suppressed.")
		return nil
	}

	// Append or extend lint_ignore
	if strings.Contains(content, "lint_ignore:") {
		// Append new entries after the existing lint_ignore line
		var lines []string
		for _, check := range newChecks {
			lines = append(lines, fmt.Sprintf("  - %s", check))
		}
		// Find the last lint_ignore entry and insert after it
		contentLines := strings.Split(content, "\n")
		var result []string
		lastIgnoreIdx := -1
		for i, line := range contentLines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") && lastIgnoreIdx == i-1 {
				lastIgnoreIdx = i
			}
			if trimmed == "lint_ignore:" {
				lastIgnoreIdx = i
			}
		}
		for i, line := range contentLines {
			result = append(result, line)
			if i == lastIgnoreIdx {
				result = append(result, lines...)
			}
		}
		content = strings.Join(result, "\n")
	} else {
		// Add new lint_ignore section
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "lint_ignore:\n"
		for _, check := range newChecks {
			content += fmt.Sprintf("  - %s\n", check)
		}
	}

	if err := os.WriteFile(appYAMLPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing app.yaml: %w", err)
	}

	fmt.Printf("Suppressed %d check(s) in %s:\n", len(newChecks), appYAMLPath)
	for _, check := range newChecks {
		fmt.Printf("  - %s\n", check)
	}
	return nil
}

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func init() {
	rootCmd.AddCommand(docsCmd)
	docsCmd.AddCommand(docsManCmd)
	docsCmd.AddCommand(docsMarkdownCmd)
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate documentation",
	Long:  "Generate man pages or markdown documentation for homelabctl.",
}

var docsManCmd = &cobra.Command{
	Use:   "man",
	Short: "Generate man pages",
	Long:  "Generate man pages to the ./man/ directory.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "./man"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating man directory: %w", err)
		}

		header := &doc.GenManHeader{
			Title:   "HOMELABCTL",
			Section: "1",
		}
		if err := doc.GenManTree(rootCmd, header, dir); err != nil {
			return fmt.Errorf("generating man pages: %w", err)
		}

		fmt.Printf("Man pages generated in %s/\n", dir)
		return nil
	},
}

var docsMarkdownCmd = &cobra.Command{
	Use:   "markdown",
	Short: "Generate markdown documentation",
	Long:  "Generate markdown documentation to the ./docs/ directory.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "./docs"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating docs directory: %w", err)
		}

		if err := doc.GenMarkdownTree(rootCmd, dir); err != nil {
			return fmt.Errorf("generating markdown docs: %w", err)
		}

		fmt.Printf("Markdown docs generated in %s/\n", dir)
		return nil
	},
}

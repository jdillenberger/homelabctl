package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// outdatedResult holds the result for a single image check.
type outdatedResult struct {
	App     string              `json:"app"`
	Image   string              `json:"image"`
	Current string              `json:"current"`
	Updates []app.VersionUpdate `json:"updates,omitempty"`
	Error   string              `json:"error,omitempty"`
}

var appsOutdatedCmd = &cobra.Command{
	Use:   "outdated [app]",
	Short: "Check for newer container image versions",
	Long: `Scan deployed apps for container images with newer versions available
in their registries. Only checks images with semver tags.

Without arguments, checks all deployed apps. Specify an app name to check
only that app.`,
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

		// Determine which apps to check
		var appsToCheck []string
		if len(args) > 0 {
			appsToCheck = []string{args[0]}
		} else {
			deployed, err := mgr.ListDeployed()
			if err != nil {
				return err
			}
			appsToCheck = deployed
		}

		if len(appsToCheck) == 0 {
			fmt.Println("No apps deployed.")
			return nil
		}

		// Collect images from deployed compose files
		type imageCheck struct {
			appName string
			ref     app.ImageRef
			image   string
		}
		var checks []imageCheck

		for _, appName := range appsToCheck {
			composePath := filepath.Join(cfg.AppDir(appName), "docker-compose.yml")
			data, err := os.ReadFile(composePath)
			if err != nil {
				continue
			}
			refs, err := app.ScanDeployedImages(data)
			if err != nil {
				continue
			}
			for _, ref := range refs {
				// Only check semver-tagged images
				if _, err := app.ParseSemver(ref.Tag); err != nil {
					continue
				}
				checks = append(checks, imageCheck{
					appName: appName,
					ref:     ref,
					image:   ref.String(),
				})
			}
		}

		if len(checks) == 0 {
			fmt.Println("No semver-tagged images found in deployed apps.")
			return nil
		}

		// Query registries in parallel (bounded concurrency)
		resolver := app.NewImageResolver()
		results := make([]outdatedResult, len(checks))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5) // max 5 concurrent requests

		for i, chk := range checks {
			wg.Add(1)
			go func(idx int, c imageCheck) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				r := outdatedResult{
					App:     c.appName,
					Image:   c.image,
					Current: c.ref.Tag,
				}

				updates, err := resolver.FindNewerVersions(c.ref)
				if err != nil {
					r.Error = err.Error()
				} else {
					r.Updates = updates
				}
				results[idx] = r
			}(i, chk)
		}
		wg.Wait()

		// Filter to only results with updates (unless there's an error)
		var withUpdates []outdatedResult
		for _, r := range results {
			if len(r.Updates) > 0 || r.Error != "" {
				withUpdates = append(withUpdates, r)
			}
		}

		if jsonOutput {
			return outputJSON(results)
		}

		if len(withUpdates) == 0 {
			fmt.Println("All images are up to date.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "APP\tIMAGE\tCURRENT\tLATEST\tTYPE")
		for _, r := range withUpdates {
			if r.Error != "" {
				fmt.Fprintf(w, "%s\t%s\t%s\t-\terror: %s\n", r.App, r.Image, r.Current, r.Error)
				continue
			}
			for _, u := range r.Updates {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.App, r.Image, u.CurrentTag, u.NewTag, u.Type)
			}
		}
		w.Flush()
		return nil
	},
}

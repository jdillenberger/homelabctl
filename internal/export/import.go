package export

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/app"
)

// ParseExportFile reads and parses a homelab export YAML file.
func ParseExportFile(path string) (*HomelabExport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading export file: %w", err)
	}

	var export HomelabExport
	if err := yaml.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("parsing export file: %w", err)
	}

	if export.Version == "" {
		return nil, fmt.Errorf("export file missing version field")
	}

	return &export, nil
}

// Import deploys apps from an export file that are not already deployed.
func Import(export *HomelabExport, mgr *app.Manager, dryRun bool) error {
	deployed, err := mgr.ListDeployed()
	if err != nil {
		return fmt.Errorf("listing deployed apps: %w", err)
	}

	deployedSet := make(map[string]bool)
	for _, d := range deployed {
		deployedSet[d] = true
	}

	for _, appExport := range export.Apps {
		if deployedSet[appExport.Name] {
			fmt.Printf("  %s: already deployed, skipping\n", appExport.Name)
			continue
		}

		// Validate template exists
		if _, ok := mgr.Registry().Get(appExport.Template); !ok {
			fmt.Printf("  %s: template %q not found, skipping\n", appExport.Name, appExport.Template)
			continue
		}

		if dryRun {
			fmt.Printf("  %s: would deploy (template: %s, version: %s)\n", appExport.Name, appExport.Template, appExport.Version)
			continue
		}

		fmt.Printf("  %s: deploying...\n", appExport.Name)
		if err := mgr.Deploy(appExport.Template, app.DeployOptions{
			Values:  appExport.Values,
			DryRun:  false,
			Confirm: true,
		}); err != nil {
			fmt.Printf("  %s: deploy failed: %v\n", appExport.Name, err)
			continue
		}
		fmt.Printf("  %s: deployed successfully\n", appExport.Name)
	}

	return nil
}

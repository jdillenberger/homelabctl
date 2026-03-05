package export

import (
	"fmt"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// HomelabExport represents the full homelab configuration for export/import.
type HomelabExport struct {
	Version string        `yaml:"version"`
	Config  config.Config `yaml:"config"`
	Apps    []AppExport   `yaml:"apps"`
}

// AppExport represents a single app's deployment info for export.
type AppExport struct {
	Name     string            `yaml:"name"`
	Template string            `yaml:"template"`
	Version  string            `yaml:"version"`
	Values   map[string]string `yaml:"values,omitempty"`
}

// BuildExport creates a HomelabExport from the current config and deployed apps.
func BuildExport(cfg *config.Config, mgr *app.Manager) (*HomelabExport, error) {
	deployed, err := mgr.ListDeployed()
	if err != nil {
		return nil, fmt.Errorf("listing deployed apps: %w", err)
	}

	var apps []AppExport
	for _, name := range deployed {
		info, err := mgr.GetDeployedInfo(name)
		if err != nil {
			continue
		}

		// Filter out system-generated values
		values := make(map[string]string)
		for k, v := range info.Values {
			switch k {
			case "hostname", "domain", "data_dir", "app_name", "network":
				continue
			default:
				values[k] = v
			}
		}

		apps = append(apps, AppExport{
			Name:     info.Name,
			Template: info.Template,
			Version:  info.Version,
			Values:   values,
		})
	}

	return &HomelabExport{
		Version: "1",
		Config:  *cfg,
		Apps:    apps,
	}, nil
}

package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"path/filepath"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

func init() {
	rootCmd.AddCommand(routingCmd)
	routingCmd.AddCommand(routingListCmd)
	routingCmd.AddCommand(routingStatusCmd)
	routingCmd.AddCommand(routingAddCmd)
	routingCmd.AddCommand(routingRemoveCmd)
	routingCmd.AddCommand(routingEnableCmd)
	routingCmd.AddCommand(routingDisableCmd)

	routingAddCmd.ValidArgsFunction = completeDeployedApps
	routingRemoveCmd.ValidArgsFunction = completeDeployedApps
	routingEnableCmd.ValidArgsFunction = completeDeployedApps
	routingDisableCmd.ValidArgsFunction = completeDeployedApps
}

var routingCmd = &cobra.Command{
	Use:   "routing",
	Short: "Manage app routing and domains",
	Long:  "Manage reverse proxy routing configuration and domain mappings for deployed apps.",
}

var routingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List routing configuration for all apps",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		mgr, err := newManager()
		if err != nil {
			return err
		}

		deployed, err := mgr.ListDeployed()
		if err != nil {
			return err
		}

		if len(deployed) == 0 {
			fmt.Println("No apps deployed.")
			return nil
		}

		type routingEntry struct {
			App     string `json:"app"`
			Enabled bool   `json:"enabled"`
			Domains string `json:"domains"`
			Port    int    `json:"port"`
		}

		var entries []routingEntry
		for _, name := range deployed {
			info, err := mgr.GetDeployedInfo(name)
			if err != nil {
				continue
			}
			entry := routingEntry{App: name}
			if info.Routing != nil {
				entry.Enabled = info.Routing.Enabled
				entry.Domains = strings.Join(info.Routing.Domains, ", ")
				entry.Port = info.Routing.ContainerPort
			}
			entries = append(entries, entry)
		}

		if jsonOutput {
			return outputJSON(entries)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "APP\tENABLED\tDOMAINS\tPORT")
		for _, e := range entries {
			enabled := "no"
			if e.Enabled {
				enabled = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", e.App, enabled, e.Domains, e.Port)
		}
		w.Flush()

		if !cfg.Routing.Enabled {
			fmt.Println("\nNote: routing is not enabled. Run 'homelabctl config set routing.enabled true' to enable.")
		}

		return nil
	},
}

var routingStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show routing system status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		fmt.Printf("Routing enabled:  %v\n", cfg.Routing.Enabled)
		fmt.Printf("Provider:         %s\n", cfg.Routing.Provider)
		fmt.Printf("Domain:           %s\n", cfg.RoutingDomain())
		fmt.Printf("HTTPS enabled:    %v\n", cfg.Routing.HTTPS.Enabled)
		if cfg.Routing.HTTPS.Enabled {
			if cfg.Routing.HTTPS.AcmeEmail != "" {
				fmt.Printf("ACME email:       %s\n", cfg.Routing.HTTPS.AcmeEmail)
			}
			caCertPath := filepath.Join(cfg.DataPath("traefik"), "certs", "ca.crt")
			fmt.Printf("CA certificate:   %s\n", caCertPath)
			fmt.Printf("\nTo trust the local CA on this machine:\n")
			fmt.Printf("  sudo cp %s /usr/local/share/ca-certificates/homelabctl-ca.crt && sudo update-ca-certificates\n", caCertPath)
		}

		if cfg.Routing.Enabled {
			// Check if Traefik is deployed
			mgr, err := newManager()
			if err != nil {
				return err
			}
			deployed, _ := mgr.ListDeployed()
			traefikDeployed := false
			for _, d := range deployed {
				if d == "traefik" {
					traefikDeployed = true
					break
				}
			}
			if traefikDeployed {
				fmt.Printf("Traefik:          deployed\n")
			} else {
				fmt.Printf("Traefik:          not deployed (run 'homelabctl apps deploy traefik')\n")
			}
		}

		return nil
	},
}

var routingAddCmd = &cobra.Command{
	Use:   "add <app> <domain>",
	Short: "Add a domain to an app's routing",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName, domain := args[0], args[1]
		return modifyRouting(appName, func(info *app.DeployedApp) error {
			if info.Routing == nil {
				return fmt.Errorf("app %s has no routing configuration", appName)
			}
			for _, d := range info.Routing.Domains {
				if d == domain {
					return fmt.Errorf("domain %s already configured for %s", domain, appName)
				}
			}
			info.Routing.Domains = append(info.Routing.Domains, domain)
			fmt.Printf("Added domain %s to %s\n", domain, appName)
			return nil
		})
	},
}

var routingRemoveCmd = &cobra.Command{
	Use:   "remove <app> <domain>",
	Short: "Remove a domain from an app's routing",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName, domain := args[0], args[1]
		return modifyRouting(appName, func(info *app.DeployedApp) error {
			if info.Routing == nil {
				return fmt.Errorf("app %s has no routing configuration", appName)
			}
			var newDomains []string
			found := false
			for _, d := range info.Routing.Domains {
				if d == domain {
					found = true
				} else {
					newDomains = append(newDomains, d)
				}
			}
			if !found {
				return fmt.Errorf("domain %s not found in %s", domain, appName)
			}
			info.Routing.Domains = newDomains
			fmt.Printf("Removed domain %s from %s\n", domain, appName)
			return nil
		})
	},
}

var routingEnableCmd = &cobra.Command{
	Use:   "enable <app>",
	Short: "Enable routing for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		return modifyRouting(appName, func(info *app.DeployedApp) error {
			if info.Routing == nil {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				mgr, err := newManager()
				if err != nil {
					return err
				}
				meta, ok := mgr.Registry().Get(appName)
				if !ok {
					return fmt.Errorf("unknown app template: %s", appName)
				}
				info.Routing = computeDefaultRouting(cfg, appName, meta)
			}
			info.Routing.Enabled = true
			fmt.Printf("Routing enabled for %s\n", appName)
			return nil
		})
	},
}

var routingDisableCmd = &cobra.Command{
	Use:   "disable <app>",
	Short: "Disable routing for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		return modifyRouting(appName, func(info *app.DeployedApp) error {
			if info.Routing == nil {
				info.Routing = &app.DeployedRouting{}
			}
			info.Routing.Enabled = false
			fmt.Printf("Routing disabled for %s\n", appName)
			return nil
		})
	},
}

// modifyRouting loads a deployed app, applies the given modification function,
// saves the updated info, regenerates the compose file, and restarts the app.
func modifyRouting(appName string, modFn func(*app.DeployedApp) error) error {
	mgr, err := newManager()
	if err != nil {
		return err
	}

	info, err := mgr.GetDeployedInfo(appName)
	if err != nil {
		return fmt.Errorf("app %s is not deployed", appName)
	}

	if err := modFn(info); err != nil {
		return err
	}

	if err := mgr.SaveDeployedInfo(appName, info); err != nil {
		return err
	}

	if err := mgr.RegenerateCompose(appName); err != nil {
		return fmt.Errorf("regenerating compose: %w", err)
	}

	return nil
}

// computeDefaultRouting builds a DeployedRouting using config defaults.
func computeDefaultRouting(cfg *config.Config, appName string, meta *app.AppMeta) *app.DeployedRouting {
	routing := &app.DeployedRouting{
		Enabled:   true,
		Domains:   []string{appName + "." + cfg.RoutingDomain()},
		KeepPorts: true,
	}

	if len(meta.Ports) > 0 {
		routing.ContainerPort = meta.Ports[0].Container
	} else {
		routing.ContainerPort = 80
	}

	return routing
}

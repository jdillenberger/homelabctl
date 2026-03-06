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
	rootCmd.AddCommand(ingressCmd)
	ingressCmd.AddCommand(ingressListCmd)
	ingressCmd.AddCommand(ingressStatusCmd)
	ingressCmd.AddCommand(ingressAddCmd)
	ingressCmd.AddCommand(ingressRemoveCmd)
	ingressCmd.AddCommand(ingressEnableCmd)
	ingressCmd.AddCommand(ingressDisableCmd)

	ingressAddCmd.ValidArgsFunction = completeDeployedApps
	ingressRemoveCmd.ValidArgsFunction = completeDeployedApps
	ingressEnableCmd.ValidArgsFunction = completeDeployedApps
	ingressDisableCmd.ValidArgsFunction = completeDeployedApps
}

var ingressCmd = &cobra.Command{
	Use:   "ingress",
	Short: "Manage app ingress and domains",
	Long:  "Manage reverse proxy ingress configuration and domain mappings for deployed apps.",
}

var ingressListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ingress configuration for all apps",
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

		type ingressEntry struct {
			App     string `json:"app"`
			Enabled bool   `json:"enabled"`
			Domains string `json:"domains"`
			Port    int    `json:"port"`
		}

		var entries []ingressEntry
		for _, name := range deployed {
			info, err := mgr.GetDeployedInfo(name)
			if err != nil {
				continue
			}
			entry := ingressEntry{App: name}
			if info.Ingress != nil {
				entry.Enabled = info.Ingress.Enabled
				entry.Domains = strings.Join(info.Ingress.Domains, ", ")
				entry.Port = info.Ingress.ContainerPort
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

		if !cfg.Ingress.Enabled {
			fmt.Println("\nNote: ingress is not enabled. Run 'homelabctl config set ingress.enabled true' to enable.")
		}

		return nil
	},
}

var ingressStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show ingress system status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		fmt.Printf("Ingress enabled:  %v\n", cfg.Ingress.Enabled)
		fmt.Printf("Provider:         %s\n", cfg.Ingress.Provider)
		fmt.Printf("Domain:           %s\n", cfg.IngressDomain())
		fmt.Printf("HTTPS enabled:    %v\n", cfg.Ingress.HTTPS.Enabled)
		if cfg.Ingress.HTTPS.Enabled {
			if cfg.Ingress.HTTPS.AcmeEmail != "" {
				fmt.Printf("ACME email:       %s\n", cfg.Ingress.HTTPS.AcmeEmail)
			}
			caCertPath := filepath.Join(cfg.DataPath("traefik"), "certs", "ca.crt")
			fmt.Printf("CA certificate:   %s\n", caCertPath)
			fmt.Printf("\nTo trust the local CA on this machine:\n")
			fmt.Printf("  sudo cp %s /usr/local/share/ca-certificates/homelabctl-ca.crt && sudo update-ca-certificates\n", caCertPath)
		}

		if cfg.Ingress.Enabled {
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

var ingressAddCmd = &cobra.Command{
	Use:   "add <app> <domain>",
	Short: "Add a domain to an app's ingress",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName, domain := args[0], args[1]
		return modifyIngress(appName, func(info *app.DeployedApp) error {
			if info.Ingress == nil {
				return fmt.Errorf("app %s has no ingress configuration", appName)
			}
			for _, d := range info.Ingress.Domains {
				if d == domain {
					return fmt.Errorf("domain %s already configured for %s", domain, appName)
				}
			}
			info.Ingress.Domains = append(info.Ingress.Domains, domain)
			fmt.Printf("Added domain %s to %s\n", domain, appName)
			return nil
		})
	},
}

var ingressRemoveCmd = &cobra.Command{
	Use:   "remove <app> <domain>",
	Short: "Remove a domain from an app's ingress",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName, domain := args[0], args[1]
		return modifyIngress(appName, func(info *app.DeployedApp) error {
			if info.Ingress == nil {
				return fmt.Errorf("app %s has no ingress configuration", appName)
			}
			var newDomains []string
			found := false
			for _, d := range info.Ingress.Domains {
				if d == domain {
					found = true
				} else {
					newDomains = append(newDomains, d)
				}
			}
			if !found {
				return fmt.Errorf("domain %s not found in %s", domain, appName)
			}
			info.Ingress.Domains = newDomains
			fmt.Printf("Removed domain %s from %s\n", domain, appName)
			return nil
		})
	},
}

var ingressEnableCmd = &cobra.Command{
	Use:   "enable <app>",
	Short: "Enable ingress for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		return modifyIngress(appName, func(info *app.DeployedApp) error {
			if info.Ingress == nil {
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
				info.Ingress = computeDefaultIngress(cfg, appName, meta)
			}
			info.Ingress.Enabled = true
			fmt.Printf("Ingress enabled for %s\n", appName)
			return nil
		})
	},
}

var ingressDisableCmd = &cobra.Command{
	Use:   "disable <app>",
	Short: "Disable ingress for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		return modifyIngress(appName, func(info *app.DeployedApp) error {
			if info.Ingress == nil {
				info.Ingress = &app.DeployedIngress{}
			}
			info.Ingress.Enabled = false
			fmt.Printf("Ingress disabled for %s\n", appName)
			return nil
		})
	},
}

// modifyIngress loads a deployed app, applies the given modification function,
// saves the updated info, regenerates the compose file, and restarts the app.
func modifyIngress(appName string, modFn func(*app.DeployedApp) error) error {
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

// computeDefaultIngress builds a DeployedIngress using config defaults.
func computeDefaultIngress(cfg *config.Config, appName string, meta *app.AppMeta) *app.DeployedIngress {
	ingress := &app.DeployedIngress{
		Enabled:   true,
		Domains:   []string{appName + "." + cfg.IngressDomain()},
		KeepPorts: true,
	}

	if len(meta.Ports) > 0 {
		ingress.ContainerPort = meta.Ports[0].Container
	} else {
		ingress.ContainerPort = 80
	}

	return ingress
}

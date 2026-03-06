package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/mdns"
)

func init() {
	rootCmd.AddCommand(fleetCmd)
	fleetCmd.AddCommand(fleetStatusCmd)
	fleetCmd.AddCommand(fleetDiscoverCmd)

	fleetDiscoverCmd.Flags().DurationP("timeout", "t", 5*time.Second, "mDNS discovery timeout")
}

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Manage the homelabctl fleet",
	Long:  "Discover peers, view fleet status, and coordinate deployments across hosts.",
}

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet status",
	Long:  "Display all known fleet hosts with their apps and online status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fleetCfg, err := config.LoadFleetConfig()
		if err != nil {
			return fmt.Errorf("loading fleet config: %w", err)
		}

		fmt.Printf("Fleet: %s\n\n", fleetCfg.Fleet.Name)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "HOSTNAME\tROLE\tADDRESS\tPORT\tAPPS\tSTATUS")

		for _, host := range fleetCfg.Hosts {
			status := "configured"
			if host.Online {
				status = "online"
			}
			apps := strings.Join(host.Apps, ", ")
			if apps == "" {
				apps = "-"
			}
			addr := host.Address
			if addr == "" {
				addr = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
				host.Hostname, host.Role, addr, host.Port, apps, status)
		}
		w.Flush()
		return nil
	},
}

var fleetDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover fleet peers via mDNS",
	Long:  "Run one-shot mDNS discovery to find homelabctl peers on the local network.",
	RunE: func(cmd *cobra.Command, args []string) error {
		timeout, _ := cmd.Flags().GetDuration("timeout")

		fmt.Printf("Discovering peers (timeout: %s)...\n", timeout)

		hosts, err := mdns.Discover(timeout)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		if len(hosts) == 0 {
			fmt.Println("No peers found.")
			return nil
		}

		fmt.Printf("Found %d peer(s):\n\n", len(hosts))

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "HOSTNAME\tROLE\tADDRESS\tPORT\tVERSION\tAPPS")

		for _, host := range hosts {
			apps := strings.Join(host.Apps, ", ")
			if apps == "" {
				apps = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
				host.Hostname, host.Role, host.Address, host.Port, host.Version, apps)
		}
		w.Flush()
		return nil
	},
}

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	fleetCmd.AddCommand(fleetSyncCmd)
	fleetCmd.AddCommand(fleetDeployCmd)

	fleetDiscoverCmd.Flags().DurationP("timeout", "t", 5*time.Second, "mDNS discovery timeout")
	fleetDeployCmd.Flags().String("host", "", "Target hostname to deploy on")
	_ = fleetDeployCmd.MarkFlagRequired("host")
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

var fleetSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync fleet config to discovered peers",
	Long:  "Push the local fleet configuration to all discovered peers via their HTTP API.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fleetCfg, err := config.LoadFleetConfig()
		if err != nil {
			return fmt.Errorf("loading fleet config: %w", err)
		}

		// Discover peers
		fmt.Println("Discovering peers...")
		hosts, err := mdns.Discover(5 * time.Second)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		if len(hosts) == 0 {
			fmt.Println("No peers found to sync with.")
			return nil
		}

		// Serialize fleet config
		payload, err := json.Marshal(fleetCfg)
		if err != nil {
			return fmt.Errorf("marshaling fleet config: %w", err)
		}

		// Push to each peer
		client := &http.Client{Timeout: 10 * time.Second}
		for _, host := range hosts {
			url := fmt.Sprintf("http://%s:%d/api/fleet", host.Address, host.Port)
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: failed to create request: %v\n", host.Hostname, err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if fleetCfg.Fleet.Secret != "" {
				req.Header.Set("X-Fleet-Secret", fleetCfg.Fleet.Secret)
			}

			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: sync failed: %v\n", host.Hostname, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				fmt.Printf("  %s: synced OK\n", host.Hostname)
			} else {
				fmt.Fprintf(os.Stderr, "  %s: sync returned %s\n", host.Hostname, resp.Status)
			}
		}

		return nil
	},
}

var fleetDeployCmd = &cobra.Command{
	Use:   "deploy <app>",
	Short: "Deploy an app on a remote host",
	Long:  "Deploy an app on a remote fleet host via its HTTP API.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		targetHost, _ := cmd.Flags().GetString("host")

		fleetCfg, err := config.LoadFleetConfig()
		if err != nil {
			return fmt.Errorf("loading fleet config: %w", err)
		}

		// Find the target host in fleet config or via discovery
		var target *config.FleetHost
		for i := range fleetCfg.Hosts {
			if fleetCfg.Hosts[i].Hostname == targetHost {
				target = &fleetCfg.Hosts[i]
				break
			}
		}

		// If not found in config, try mDNS discovery
		if target == nil {
			fmt.Printf("Host %q not in fleet config, trying mDNS discovery...\n", targetHost)
			hosts, err := mdns.Discover(5 * time.Second)
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}
			for i := range hosts {
				if hosts[i].Hostname == targetHost {
					target = &hosts[i]
					break
				}
			}
		}

		if target == nil {
			return fmt.Errorf("host %q not found in fleet config or via mDNS", targetHost)
		}
		if target.Address == "" {
			return fmt.Errorf("host %q has no known address", targetHost)
		}

		// Build deploy request
		deployReq := struct {
			App string `json:"app"`
		}{
			App: appName,
		}
		payload, err := json.Marshal(deployReq)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}

		url := fmt.Sprintf("http://%s:%d/api/fleet/deploy", target.Address, target.Port)
		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if fleetCfg.Fleet.Secret != "" {
			req.Header.Set("X-Fleet-Secret", fleetCfg.Fleet.Secret)
		}

		fmt.Printf("Deploying %s on %s (%s:%d)...\n", appName, targetHost, target.Address, target.Port)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("deploy request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Deploy of %s on %s succeeded.\n", appName, targetHost)
		} else {
			var errResp struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("deploy failed (HTTP %d): %s", resp.StatusCode, errResp.Error)
		}

		return nil
	},
}

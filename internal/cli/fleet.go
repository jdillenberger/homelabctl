package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/peerscan"
)

func init() {
	rootCmd.AddCommand(fleetCmd)
	fleetCmd.AddCommand(fleetStatusCmd)
}

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Manage the homelabctl fleet",
	Long:  "View fleet status from the local peer-scanner daemon.",
}

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet status",
	Long:  "Query the local peer-scanner daemon and display all known peers.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		client := peerscan.NewClient(cfg.PeerScanner.URL, cfg.PeerScanner.Secret)
		resp, err := client.Peers()
		if err != nil {
			return fmt.Errorf("querying peer-scanner: %w", err)
		}

		fmt.Printf("Fleet: %s\n\n", resp.Fleet.Name)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "HOSTNAME\tROLE\tADDRESS\tPORT\tVERSION\tSTATUS")

		// Show self first
		printPeerRow(w, resp.Self)

		for _, peer := range resp.Peers {
			printPeerRow(w, peer)
		}
		w.Flush()
		return nil
	},
}

func printPeerRow(w *tabwriter.Writer, p peerscan.Peer) {
	status := "offline"
	if p.Online {
		status = "online"
	}
	addr := p.Address
	if addr == "" {
		addr = "-"
	}
	version := p.Version
	if version == "" {
		version = "-"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
		p.Hostname, p.Role, addr, p.Port, version, status)
}

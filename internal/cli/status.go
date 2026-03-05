package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/stats"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system and app status dashboard",
	Long:  "Display a compact overview of system resources and all running apps.",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		// Collect system stats
		s, err := stats.Collect()
		if err != nil {
			return err
		}

		// Collect app statuses
		deployed, err := mgr.ListDeployed()
		if err != nil {
			return err
		}

		type appEntry struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Status  string `json:"status"`
		}

		var appEntries []appEntry
		for _, name := range deployed {
			info, _ := mgr.GetDeployedInfo(name)
			ver := ""
			if info != nil {
				ver = info.Version
			}
			status, err := mgr.Status(name)
			if err != nil {
				appEntries = append(appEntries, appEntry{Name: name, Version: ver, Status: "error"})
			} else if status == "" {
				appEntries = append(appEntries, appEntry{Name: name, Version: ver, Status: "stopped"})
			} else {
				appEntries = append(appEntries, appEntry{Name: name, Version: ver, Status: "running"})
			}
		}

		if jsonOutput {
			return outputJSON(map[string]interface{}{
				"system": s,
				"apps":   appEntries,
			})
		}

		// System section
		uptime := time.Duration(s.Uptime) * time.Second
		days := int(uptime.Hours()) / 24
		hours := int(uptime.Hours()) % 24
		mins := int(uptime.Minutes()) % 60

		fmt.Println("System")
		fmt.Println("──────")
		fmt.Printf("  CPU:     %.1f%% (%d cores)\n", s.CPU.UsagePercent, s.CPU.Cores)
		fmt.Printf("  Memory:  %s / %s (%.1f%%)\n",
			stats.FormatBytes(s.Memory.Used),
			stats.FormatBytes(s.Memory.Total),
			s.Memory.UsedPercent)
		fmt.Printf("  Disk:    %s / %s (%.1f%%)\n",
			stats.FormatBytes(s.Disk.Used),
			stats.FormatBytes(s.Disk.Total),
			s.Disk.UsedPercent)
		fmt.Printf("  Uptime:  %dd %dh %dm\n", days, hours, mins)

		// Apps section
		fmt.Println("\nApps")
		fmt.Println("────")
		if len(appEntries) == 0 {
			fmt.Println("  No apps deployed.")
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  NAME\tVERSION\tSTATUS")
			for _, a := range appEntries {
				fmt.Fprintf(w, "  %s\t%s\t%s\n", a.Name, a.Version, a.Status)
			}
			w.Flush()
		}

		return nil
	},
}

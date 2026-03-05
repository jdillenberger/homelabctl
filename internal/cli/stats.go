package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/stats"
)

func init() {
	rootCmd.AddCommand(statsCmd)
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show system resource statistics",
	Long:  "Display CPU, memory, disk, and temperature information.",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := stats.Collect()
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(s)
		}

		// Human-readable output
		fmt.Println("System Statistics")
		fmt.Println("─────────────────")
		fmt.Printf("  CPU:         %.1f%% (%d cores)\n", s.CPU.UsagePercent, s.CPU.Cores)
		fmt.Printf("  Memory:      %s / %s (%.1f%%)\n",
			stats.FormatBytes(s.Memory.Used),
			stats.FormatBytes(s.Memory.Total),
			s.Memory.UsedPercent)
		fmt.Printf("  Disk (%s):   %s / %s (%.1f%%)\n",
			s.Disk.Path,
			stats.FormatBytes(s.Disk.Used),
			stats.FormatBytes(s.Disk.Total),
			s.Disk.UsedPercent)

		if len(s.Temperature) > 0 {
			fmt.Println("  Temperature:")
			for _, t := range s.Temperature {
				fmt.Printf("    %-20s %.1f°C\n", t.SensorKey, t.Temperature)
			}
		}

		uptime := time.Duration(s.Uptime) * time.Second
		days := int(uptime.Hours()) / 24
		hours := int(uptime.Hours()) % 24
		mins := int(uptime.Minutes()) % 60
		fmt.Printf("  Uptime:      %dd %dh %dm\n", days, hours, mins)

		return nil
	},
}

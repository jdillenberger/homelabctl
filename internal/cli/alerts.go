package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/alert"
	"github.com/jdillenberger/homelabctl/internal/config"
)

func init() {
	rootCmd.AddCommand(alertsCmd)
	alertsCmd.AddCommand(alertsListCmd)
	alertsCmd.AddCommand(alertsAddCmd)
	alertsCmd.AddCommand(alertsRemoveCmd)
	alertsCmd.AddCommand(alertsTestCmd)
	alertsCmd.AddCommand(alertsHistoryCmd)

	alertsAddCmd.Flags().String("type", "", "Rule type (disk-full, high-cpu, high-memory, high-temp, app-down, backup-failed)")
	alertsAddCmd.Flags().Float64("threshold", 0, "Threshold value (e.g. 90 for 90%)")
	alertsAddCmd.Flags().String("channel", "", "Notification channel (webhook, ntfy, gotify, email)")
	alertsAddCmd.Flags().String("app", "", "App name (for app-down rules)")
	_ = alertsAddCmd.MarkFlagRequired("type")
	_ = alertsAddCmd.MarkFlagRequired("channel")

	alertsHistoryCmd.Flags().IntP("count", "n", 20, "Number of recent alerts to show")
}

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage alert rules and notifications",
	Long:  "Configure alert rules, test notifications, and view alert history.",
}

var alertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured alert rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store := alert.NewStore(cfg.DataDir)
		rules, err := store.LoadRules()
		if err != nil {
			return err
		}

		if len(rules) == 0 {
			if jsonOutput {
				return outputJSON([]struct{}{})
			}
			fmt.Println("No alert rules configured.")
			return nil
		}

		if jsonOutput {
			return outputJSON(rules)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tTHRESHOLD\tAPP\tCHANNELS\tENABLED")
		for _, r := range rules {
			appName := r.App
			if appName == "" {
				appName = "*"
			}
			threshold := "-"
			if r.Threshold > 0 {
				threshold = fmt.Sprintf("%.0f%%", r.Threshold)
			}
			channels := ""
			for i, ch := range r.Channels {
				if i > 0 {
					channels += ","
				}
				channels += ch
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%v\n",
				r.ID[:8], r.Type, threshold, appName, channels, r.Enabled)
		}
		w.Flush()
		return nil
	},
}

var alertsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an alert rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		ruleType, _ := cmd.Flags().GetString("type")
		threshold, _ := cmd.Flags().GetFloat64("threshold")
		channel, _ := cmd.Flags().GetString("channel")
		appName, _ := cmd.Flags().GetString("app")

		// Validate rule type
		valid := false
		for _, rt := range alert.ValidRuleTypes {
			if string(rt) == ruleType {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid rule type: %s", ruleType)
		}

		rule := alert.Rule{
			ID:        uuid.New().String(),
			Type:      alert.RuleType(ruleType),
			Threshold: threshold,
			App:       appName,
			Channels:  []string{channel},
			Enabled:   true,
		}

		store := alert.NewStore(cfg.DataDir)
		if err := store.AddRule(rule); err != nil {
			return err
		}

		fmt.Printf("Alert rule added: %s (id: %s)\n", ruleType, rule.ID[:8])
		return nil
	},
}

var alertsRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove an alert rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store := alert.NewStore(cfg.DataDir)

		// Support short IDs
		rules, err := store.LoadRules()
		if err != nil {
			return err
		}

		targetID := args[0]
		for _, r := range rules {
			if r.ID == targetID || (len(targetID) >= 8 && r.ID[:len(targetID)] == targetID) {
				targetID = r.ID
				break
			}
		}

		if err := store.RemoveRule(targetID); err != nil {
			return err
		}

		fmt.Printf("Alert rule removed: %s\n", args[0])
		return nil
	},
}

var alertsTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a test notification to all channels",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store := alert.NewStore(cfg.DataDir)
		mgr := alert.NewManager(store, 0)
		mgr.RegisterNotifiers(cfg.Alerts.Channels)

		if err := mgr.SendTest(); err != nil {
			return fmt.Errorf("test notification failed: %w", err)
		}

		fmt.Println("Test notification sent to all configured channels.")
		return nil
	},
}

var alertsHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent alert history",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		count, _ := cmd.Flags().GetInt("count")

		store := alert.NewStore(cfg.DataDir)
		history, err := store.LoadHistory()
		if err != nil {
			return err
		}

		if len(history) == 0 {
			if jsonOutput {
				return outputJSON([]struct{}{})
			}
			fmt.Println("No alert history.")
			return nil
		}

		// Show most recent entries
		start := 0
		if len(history) > count {
			start = len(history) - count
		}
		recent := history[start:]

		if jsonOutput {
			return outputJSON(recent)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tSEVERITY\tTYPE\tMESSAGE")
		for _, a := range recent {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				a.Timestamp.Format(time.DateTime), a.Severity, a.Type, a.Message)
		}
		w.Flush()
		return nil
	},
}

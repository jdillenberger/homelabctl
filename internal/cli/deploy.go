package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/wizard"
)

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringP("values", "f", "", "YAML file with template values")
	deployCmd.Flags().StringSlice("set", nil, "Set values (key=value)")
	deployCmd.Flags().Bool("dry-run", false, "Show rendered files without deploying")
	deployCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	deployCmd.Flags().Bool("quick", false, "Accept all defaults and auto-generate secrets")
	deployCmd.ValidArgsFunction = completeTemplateNames
}

var deployCmd = &cobra.Command{
	Use:   "deploy <app>",
	Short: "Deploy an app (shortcut for 'apps deploy')",
	Long:  "Deploy an app from a built-in or local template. Shortcut for 'homelabctl apps deploy'.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := newManager()
		if err != nil {
			return err
		}

		appName := args[0]
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		quick, _ := cmd.Flags().GetBool("quick")

		// Parse --values / -f file
		values := make(map[string]string)
		valuesFile, _ := cmd.Flags().GetString("values")
		if valuesFile != "" {
			data, err := os.ReadFile(valuesFile)
			if err != nil {
				return fmt.Errorf("reading values file: %w", err)
			}
			if err := yaml.Unmarshal(data, &values); err != nil {
				return fmt.Errorf("parsing values file: %w", err)
			}
		}

		// Parse --set values (takes precedence over file values)
		setValues, _ := cmd.Flags().GetStringSlice("set")
		for _, kv := range setValues {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --set value: %q (expected key=value)", kv)
			}
			values[parts[0]] = parts[1]
		}

		// --quick: skip wizard, accept all defaults, auto-generate secrets
		if quick {
			return mgr.Deploy(appName, app.DeployOptions{
				Values:  values,
				DryRun:  dryRun,
				Confirm: true,
			})
		}

		// If no values provided and stdin is a terminal, offer the interactive wizard
		if len(values) == 0 && !dryRun {
			if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
				meta, ok := mgr.Registry().Get(appName)
				if ok && len(meta.Values) > 0 {
					wizardValues, err := wizard.RunDeployWizard(meta)
					if err != nil {
						return err
					}
					values = wizardValues
				}
			}
		}

		yes, _ := cmd.Flags().GetBool("yes")
		return mgr.Deploy(appName, app.DeployOptions{
			Values:  values,
			DryRun:  dryRun,
			Confirm: yes,
		})
	},
}

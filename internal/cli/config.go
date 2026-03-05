package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configValidateCmd)

	configInitCmd.Flags().StringP("path", "p", "/etc/homelabctl/config.yaml", "Path for the config file")
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  "Show, set, initialize, and validate homelabctl configuration.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current configuration as YAML",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling config: %w", err)
		}

		if f := viper.ConfigFileUsed(); f != "" {
			fmt.Printf("# Config file: %s\n", f)
		}
		fmt.Print(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long:  "Set a configuration key to a value and write to the config file.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		viper.Set(key, value)

		cfgPath := viper.ConfigFileUsed()
		if cfgPath == "" {
			cfgPath = "/etc/homelabctl/config.yaml"
		}

		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		if err := viper.WriteConfigAs(cfgPath); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		fmt.Printf("Set %s = %s (written to %s)\n", key, value, cfgPath)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, _ := cmd.Flags().GetString("path")

		// Check if file already exists
		if _, err := os.Stat(cfgPath); err == nil {
			return fmt.Errorf("config file already exists at %s (remove it first or use 'config set' to modify)", cfgPath)
		}

		cfg := config.DefaultConfig()
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling default config: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			return fmt.Errorf("writing config file: %w", err)
		}

		fmt.Printf("Default config written to %s\n", cfgPath)
		return nil
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config load error: %w", err)
		}

		errors := config.Validate(cfg)
		if len(errors) == 0 {
			fmt.Println("Configuration is valid.")
			if f := viper.ConfigFileUsed(); f != "" {
				fmt.Printf("Config file: %s\n", f)
			}
			return nil
		}

		fmt.Println("Configuration errors:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("config validation failed with %d error(s)", len(errors))
	},
}

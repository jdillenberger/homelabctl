package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/logging"
)

var (
	cfgFile    string
	appsDir    string
	verbose    bool
	quiet      bool
	jsonOutput bool
	version    = "dev"
	commit     = "none"
	buildDate  = "unknown"
)

// SetVersionInfo sets build information from ldflags.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	buildDate = d
}

var rootCmd = &cobra.Command{
	Use:          "homelabctl",
	Short:        "Homelab app deployment & management tool",
	Long:         "homelabctl deploys and manages self-hosted apps on your homelab using Docker Compose.",
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Setup(verbose, quiet, jsonOutput)
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default /etc/homelabctl/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&appsDir, "apps-dir", "", "apps directory override")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output as JSON")

	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	config.SetDefaults()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/etc/homelabctl")
		viper.AddConfigPath("$HOME/.config/homelabctl")
		viper.AddConfigPath(".")
	}

	viper.SetEnvPrefix("HOMELABCTL")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config: %v\n", err)
		}
	}

	// Override from flags
	if appsDir != "" {
		viper.Set("apps_dir", appsDir)
	}

	// Validate config early so misconfigurations are caught immediately
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config for validation: %v\n", err)
		return
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: configuration issues detected:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		fmt.Fprintf(os.Stderr, "Run 'homelabctl config validate' for details.\n")
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			_ = outputJSON(map[string]string{
				"version": version,
				"commit":  commit,
				"date":    buildDate,
			})
			return
		}
		fmt.Printf("homelabctl %s (commit: %s, built: %s)\n", version, commit, buildDate)
	},
}

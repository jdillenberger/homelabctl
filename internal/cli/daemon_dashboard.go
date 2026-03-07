package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/web"
)

var daemonDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Run the web dashboard",
	Long:  "Start the homelabctl web dashboard server as a standalone daemon.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		port := resolvePort(cmd)
		devMode, _ := cmd.Flags().GetBool("dev")

		runner := &exec.Runner{Verbose: verbose}
		mgr, err := newManager()
		if err != nil {
			return err
		}

		srv, err := startDashboard(cfg, mgr, runner, port, devMode)
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("Starting web dashboard on http://0.0.0.0%s\n", addr)
		if devMode {
			fmt.Println("Development mode enabled (livereload active)")
		}

		go func() {
			if err := srv.Start(addr); err != nil {
				slog.Error("Server error", "error", err)
			}
		}()

		<-ctx.Done()
		fmt.Println("\nShutting down dashboard...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	},
}

func init() {
	daemonDashboardCmd.Flags().IntP("port", "p", 0, "port to listen on (overrides config)")
	daemonDashboardCmd.Flags().Bool("dev", false, "enable development mode with livereload")
}

// resolvePort returns the web port from flags, config, or default.
func resolvePort(cmd *cobra.Command) int {
	port, _ := cmd.Flags().GetInt("port")
	if port == 0 {
		port = viper.GetInt("network.web_port")
	}
	if port == 0 {
		port = 8080
	}
	return port
}

// startDashboard creates and configures the web server.
// It regenerates the dashboard route if traefik is deployed.
// The caller is responsible for starting and shutting down the returned server.
func startDashboard(cfg *config.Config, mgr *app.Manager, runner *exec.Runner, port int, devMode bool) (*web.Server, error) {
	srv, err := web.NewServer(cfg, mgr, runner, devMode)
	if err != nil {
		return nil, fmt.Errorf("creating server: %w", err)
	}

	// Regenerate dashboard route if traefik is deployed with routing enabled.
	if cfg.Routing.Enabled {
		if _, statErr := os.Stat(cfg.AppDir("traefik")); statErr == nil {
			cfg.Network.WebPort = port
			if err := app.GenerateDashboardRoute(cfg); err != nil {
				slog.Warn("Failed to regenerate dashboard route", "error", err)
			}
		}
	}

	return srv, nil
}

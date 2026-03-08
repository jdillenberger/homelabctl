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

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
)

func init() {
	daemonCmd.Flags().IntP("port", "p", 0, "port to listen on (overrides config)")
	daemonCmd.Flags().Bool("dev", false, "enable development mode with livereload")
	daemonCmd.AddCommand(daemonDashboardCmd)
	daemonCmd.AddCommand(daemonSchedulerCmd)
	rootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the homelabctl daemon",
	Long:  "Run all homelabctl daemon components (dashboard, scheduler).",
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

		// Start scheduler
		sched, err := startScheduler(cfg, mgr, runner)
		if err != nil {
			return fmt.Errorf("starting scheduler: %w", err)
		}
		sched.Start()
		defer sched.Stop()
		fmt.Println("Scheduler started")

		// Wire dashboard route regeneration on traefik deploy
		if cfg.Routing.Enabled {
			prevOnDeploy := mgr.OnDeploy
			mgr.OnDeploy = func(appName string, routing *app.DeployedRouting) {
				if prevOnDeploy != nil {
					prevOnDeploy(appName, routing)
				}
				if appName == "traefik" {
					if err := app.GenerateDashboardRoute(cfg); err != nil {
						slog.Warn("Failed to regenerate dashboard route", "error", err)
					}
				}
			}
		}

		// Start dashboard
		srv, err := startDashboard(cfg, mgr, runner, port, devMode)
		if err != nil {
			return fmt.Errorf("starting dashboard: %w", err)
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
		fmt.Println("\nShutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	},
}

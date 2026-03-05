package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/mdns"
	"github.com/jdillenberger/homelabctl/internal/web"
)

func init() {
	serveCmd.Flags().IntP("port", "p", 0, "port to listen on (overrides config)")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web dashboard",
	Long:  "Start the homelabctl web dashboard server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port = viper.GetInt("network.web_port")
		}
		if port == 0 {
			port = 8080
		}

		runner := &exec.Runner{Verbose: verbose}
		mgr, err := newManager()
		if err != nil {
			return err
		}

		srv, err := web.NewServer(cfg, mgr, runner)
		if err != nil {
			return fmt.Errorf("creating server: %w", err)
		}

		// Start mDNS advertising
		if cfg.MDNS.Enabled {
			deployedApps, err := mgr.ListDeployed()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not list deployed apps for mDNS: %v\n", err)
				deployedApps = nil
			}
			shutdownMDNS, err := mdns.Advertise(cfg, version, deployedApps)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: mDNS advertising failed: %v\n", err)
			} else {
				defer shutdownMDNS()
			}
		}

		// Start backup scheduler
		if cfg.Backup.Enabled {
			scheduler := backup.NewScheduler()
			err := scheduler.Start(cfg.Backup.Schedule, func() {
				deployed, err := mgr.ListDeployed()
				if err != nil {
					log.Printf("Backup: failed to list deployed apps: %v\n", err)
					return
				}
				borg := backup.NewBorg(runner)
				registry := mgr.Registry()
				for _, appName := range deployed {
					configFile := backup.ConfigPath(cfg.AppsDir, appName)
					if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
						continue
					}
					meta, _ := registry.Get(appName)

					// Run pre-hook if defined
					if meta != nil && meta.Backup != nil && meta.Backup.PreHook != "" {
						if _, hookErr := runner.Run("sh", "-c", meta.Backup.PreHook); hookErr != nil {
							log.Printf("Backup: pre-hook for %s failed: %v\n", appName, hookErr)
							continue
						}
					}

					if _, borgErr := borg.Create(configFile); borgErr != nil {
						log.Printf("Backup: failed for %s: %v\n", appName, borgErr)
					}

					// Run post-hook if defined
					if meta != nil && meta.Backup != nil && meta.Backup.PostHook != "" {
						if _, hookErr := runner.Run("sh", "-c", meta.Backup.PostHook); hookErr != nil {
							log.Printf("Backup: post-hook for %s failed: %v\n", appName, hookErr)
						}
					}
				}
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: backup scheduler failed to start: %v\n", err)
			} else {
				defer scheduler.Stop()
			}
		}

		// Graceful shutdown
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("Starting web dashboard on http://0.0.0.0%s\n", addr)

		go func() {
			if err := srv.Start(addr); err != nil {
				fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			}
		}()

		<-ctx.Done()
		fmt.Println("\nShutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Echo.Shutdown(shutdownCtx)
	},
}

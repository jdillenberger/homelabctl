package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jdillenberger/homelabctl/internal/alert"
	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/mdns"
	"github.com/jdillenberger/homelabctl/internal/scheduler"
	"github.com/jdillenberger/homelabctl/internal/web"
)

func init() {
	serveCmd.Flags().IntP("port", "p", 0, "port to listen on (overrides config)")
	serveCmd.Flags().Bool("dev", false, "enable development mode with livereload")
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

		devMode, _ := cmd.Flags().GetBool("dev")

		runner := &exec.Runner{Verbose: verbose}
		mgr, err := newManager()
		if err != nil {
			return err
		}

		srv, err := web.NewServer(cfg, mgr, runner, devMode)
		if err != nil {
			return fmt.Errorf("creating server: %w", err)
		}

		// Start mDNS advertising
		if cfg.MDNS.Enabled {
			deployedApps, err := mgr.ListDeployed()
			if err != nil {
				slog.Warn("Could not list deployed apps for mDNS", "error", err)
				deployedApps = nil
			}
			shutdownMDNS, err := mdns.Advertise(cfg, version, deployedApps)
			if err != nil {
				slog.Warn("mDNS advertising failed", "error", err)
			} else {
				defer shutdownMDNS()
			}

			// Advertise app routing domains via Avahi CNAME
			if cfg.MDNS.AdvertiseApps {
				avahiMgr := mdns.NewAvahiCNAME(runner)
				for _, appName := range deployedApps {
					info, err := mgr.GetDeployedInfo(appName)
					if err != nil || info.Routing == nil || !info.Routing.Enabled {
						continue
					}
					for _, domain := range info.Routing.Domains {
						if strings.HasSuffix(domain, ".local") {
							if err := avahiMgr.PublishCNAME(appName+":"+domain, domain); err != nil {
								slog.Warn("Failed to publish CNAME", "domain", domain, "error", err)
							}
						}
					}
				}
				defer avahiMgr.Shutdown()

				// Wire Manager callbacks for dynamic CNAME management
				mgr.OnDeploy = func(appName string, routing *app.DeployedRouting) {
					if routing == nil || !routing.Enabled {
						return
					}
					for _, d := range routing.Domains {
						if strings.HasSuffix(d, ".local") {
							if err := avahiMgr.PublishCNAME(appName+":"+d, d); err != nil {
								slog.Warn("Failed to publish CNAME", "domain", d, "error", err)
							}
						}
					}
				}
				mgr.OnRemove = func(appName string) {
					for key := range avahiMgr.ListPublished() {
						if strings.HasPrefix(key, appName+":") {
							_ = avahiMgr.UnpublishCNAME(key)
						}
					}
				}
			}
		}

		// Set up generalized scheduler
		sched := scheduler.New()

		// Set up alert manager (used by multiple jobs)
		var alertMgr *alert.Manager
		if cfg.Alerts.Enabled {
			alertStore := alert.NewStore(cfg.DataDir)
			cooldown, err := time.ParseDuration(cfg.Alerts.Cooldown)
			if err != nil {
				slog.Warn("Invalid alert cooldown, using 15m", "error", err)
				cooldown = 15 * time.Minute
			}
			alertMgr = alert.NewManager(alertStore, cooldown)
			alertMgr.RegisterNotifiers(cfg.Alerts.Channels)
		}

		// Register backup job
		if cfg.Backup.Enabled {
			backupFunc := func() {
				deployed, err := mgr.ListDeployed()
				if err != nil {
					slog.Error("Backup: failed to list deployed apps", "error", err)
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
							slog.Error("Backup pre-hook failed", "app", appName, "error", hookErr)
							continue
						}
					}

					if _, borgErr := borg.Create(configFile); borgErr != nil {
						slog.Error("Backup failed", "app", appName, "error", borgErr)
						if alertMgr != nil {
							alertMgr.NotifyBackupFailed(appName, borgErr)
						}
					}

					// Run post-hook if defined
					if meta != nil && meta.Backup != nil && meta.Backup.PostHook != "" {
						if _, hookErr := runner.Run("sh", "-c", meta.Backup.PostHook); hookErr != nil {
							slog.Error("Backup post-hook failed", "app", appName, "error", hookErr)
						}
					}
				}
			}
			if err := sched.Add(scheduler.Job{
				Name:     "backup",
				Schedule: cfg.Backup.Schedule,
				Func:     backupFunc,
			}); err != nil {
				slog.Warn("Backup scheduler failed to start", "error", err)
			}
		}

		// Register health check job (checks app health, evaluates alert rules)
		if cfg.Health.Enabled {
			healthChecker := app.NewHealthChecker()
			compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)

			healthFunc := func() {
				deployed, err := mgr.ListDeployed()
				if err != nil {
					slog.Error("Health check: failed to list deployed apps", "error", err)
					return
				}

				var results []app.HealthResult
				registry := mgr.Registry()
				for _, appName := range deployed {
					meta, ok := registry.Get(appName)
					if !ok {
						continue
					}
					result := healthChecker.CheckApp(meta, compose, cfg.AppDir(appName))
					results = append(results, result)
				}

				if alertMgr != nil {
					alertMgr.Evaluate(results)
				}
			}
			if err := sched.Add(scheduler.Job{
				Name:     "health-check",
				Schedule: cfg.Health.Schedule,
				Func:     healthFunc,
			}); err != nil {
				slog.Warn("Health check scheduler failed to start", "error", err)
			}
		}

		// Register alert monitoring job (only if health is not enabled, to avoid duplicate checks)
		if cfg.Alerts.Enabled && !cfg.Health.Enabled {
			compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
			healthChecker := app.NewHealthChecker()

			alertFunc := func() {
				deployed, err := mgr.ListDeployed()
				if err != nil {
					slog.Error("Alert monitor: failed to list deployed apps", "error", err)
					return
				}

				var results []app.HealthResult
				registry := mgr.Registry()
				for _, appName := range deployed {
					meta, ok := registry.Get(appName)
					if !ok {
						continue
					}
					result := healthChecker.CheckApp(meta, compose, cfg.AppDir(appName))
					results = append(results, result)
				}

				alertMgr.Evaluate(results)
			}
			if err := sched.Add(scheduler.Job{
				Name:     "alert-monitor",
				Schedule: cfg.Alerts.Schedule,
				Func:     alertFunc,
			}); err != nil {
				slog.Warn("Alert monitor scheduler failed to start", "error", err)
			}
		}

		// Register auto-update job
		if cfg.Updates.Enabled {
			compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)
			updateFunc := func() {
				deployed, err := mgr.ListDeployed()
				if err != nil {
					slog.Error("Auto-update: failed to list deployed apps", "error", err)
					return
				}
				registry := mgr.Registry()
				for _, appName := range deployed {
					appDir := cfg.AppDir(appName)
					meta, _ := registry.Get(appName)
					if meta != nil && meta.RequiresBuild {
						slog.Info("Auto-update: building", "app", appName)
						if _, err := compose.UpWithBuild(appDir); err != nil {
							slog.Error("Auto-update: build/up failed", "app", appName, "error", err)
							if alertMgr != nil {
								alertMgr.NotifyUpdateFailed(appName, err)
							}
						}
					} else {
						slog.Info("Auto-update: pulling images", "app", appName)
						if _, err := compose.Pull(appDir); err != nil {
							slog.Error("Auto-update: pull failed", "app", appName, "error", err)
							if alertMgr != nil {
								alertMgr.NotifyUpdateFailed(appName, err)
							}
							continue
						}
						if _, err := compose.Up(appDir); err != nil {
							slog.Error("Auto-update: up failed", "app", appName, "error", err)
							if alertMgr != nil {
								alertMgr.NotifyUpdateFailed(appName, err)
							}
						}
					}
				}
			}
			if err := sched.Add(scheduler.Job{
				Name:     "auto-update",
				Schedule: cfg.Updates.Schedule,
				Func:     updateFunc,
			}); err != nil {
				slog.Warn("Auto-update scheduler failed to start", "error", err)
			}
		}

		// Register docker prune job
		if cfg.Prune.Enabled {
			pruneFunc := func() {
				slog.Info("Scheduled prune: cleaning up Docker resources")
				runtime := cfg.Docker.Runtime
				cmds := [][]string{
					{runtime, "image", "prune", "-af"},
					{runtime, "volume", "prune", "-f"},
					{runtime, "network", "prune", "-f"},
					{runtime, "builder", "prune", "-af"},
				}
				for _, c := range cmds {
					if _, err := runner.Run(c[0], c[1:]...); err != nil {
						slog.Error("Scheduled prune failed", "command", c, "error", err)
					}
				}
			}
			if err := sched.Add(scheduler.Job{
				Name:     "docker-prune",
				Schedule: cfg.Prune.Schedule,
				Func:     pruneFunc,
			}); err != nil {
				slog.Warn("Docker prune scheduler failed to start", "error", err)
			}
		}

		sched.Start()
		defer sched.Stop()

		// Graceful shutdown
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
		return srv.Echo.Shutdown(shutdownCtx)
	},
}

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/alertclient"
	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/backup"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/scheduler"
)

var daemonSchedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "Run background jobs",
	Long:  "Run scheduled background jobs (backups, health checks, auto-updates, docker prune).",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		runner := &exec.Runner{Verbose: verbose}
		mgr, err := newManager()
		if err != nil {
			return err
		}

		sched, err := startScheduler(cfg, mgr, runner)
		if err != nil {
			return err
		}
		sched.Start()
		defer sched.Stop()

		fmt.Println("Scheduler running")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		<-ctx.Done()
		fmt.Println("\nShutting down scheduler...")
		return nil
	},
}

// startScheduler creates and configures the scheduler with all enabled jobs.
// The caller is responsible for calling Start() and Stop() on the returned scheduler.
func startScheduler(cfg *config.Config, mgr *app.Manager, runner *exec.Runner) (*scheduler.Scheduler, error) {
	sched := scheduler.New()

	// Alert client for pushing events to labalert
	var alertClient *alertclient.Client
	if cfg.Labalert.URL != "" {
		alertClient = alertclient.New(cfg.Labalert.URL)
	}

	// Backup job
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

				if meta != nil && meta.Backup != nil && meta.Backup.PreHook != "" {
					if _, hookErr := runner.Run("sh", "-c", meta.Backup.PreHook); hookErr != nil {
						slog.Error("Backup pre-hook failed", "app", appName, "error", hookErr)
						continue
					}
				}

				if _, borgErr := borg.Create(configFile); borgErr != nil {
					slog.Error("Backup failed", "app", appName, "error", borgErr)
					if alertClient != nil {
						if pushErr := alertClient.PushEvent(context.Background(), alertclient.Event{
							Type:     "backup-failed",
							App:      appName,
							Message:  fmt.Sprintf("Backup failed for %s: %v", appName, borgErr),
							Severity: "critical",
						}); pushErr != nil {
							slog.Error("Failed to push backup-failed event to labalert", "error", pushErr)
						}
					}
				}

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

	// Health check job
	if cfg.Health.Enabled {
		healthChecker := app.NewHealthChecker()
		compose := app.NewCompose(runner, cfg.Docker.ComposeCommand)

		healthFunc := func() {
			deployed, err := mgr.ListDeployed()
			if err != nil {
				slog.Error("Health check: failed to list deployed apps", "error", err)
				return
			}

			registry := mgr.Registry()
			for _, appName := range deployed {
				meta, ok := registry.Get(appName)
				if !ok {
					continue
				}
				_ = healthChecker.CheckApp(meta, compose, cfg.AppDir(appName))
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

	// Auto-update job
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
						if alertClient != nil {
							if pushErr := alertClient.PushEvent(context.Background(), alertclient.Event{
								Type:     "update-failed",
								App:      appName,
								Message:  fmt.Sprintf("Auto-update failed for %s: %v", appName, err),
								Severity: "warning",
							}); pushErr != nil {
								slog.Error("Failed to push update-failed event to labalert", "error", pushErr)
							}
						}
					}
				} else {
					slog.Info("Auto-update: pulling images", "app", appName)
					if _, err := compose.Pull(appDir); err != nil {
						slog.Error("Auto-update: pull failed", "app", appName, "error", err)
						if alertClient != nil {
							if pushErr := alertClient.PushEvent(context.Background(), alertclient.Event{
								Type:     "update-failed",
								App:      appName,
								Message:  fmt.Sprintf("Auto-update pull failed for %s: %v", appName, err),
								Severity: "warning",
							}); pushErr != nil {
								slog.Error("Failed to push update-failed event to labalert", "error", pushErr)
							}
						}
						continue
					}
					if _, err := compose.Up(appDir); err != nil {
						slog.Error("Auto-update: up failed", "app", appName, "error", err)
						if alertClient != nil {
							if pushErr := alertClient.PushEvent(context.Background(), alertclient.Event{
								Type:     "update-failed",
								App:      appName,
								Message:  fmt.Sprintf("Auto-update up failed for %s: %v", appName, err),
								Severity: "warning",
							}); pushErr != nil {
								slog.Error("Failed to push update-failed event to labalert", "error", pushErr)
							}
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

	// Docker prune job
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

	return sched, nil
}

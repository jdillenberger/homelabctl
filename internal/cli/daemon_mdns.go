package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/exec"
	"github.com/jdillenberger/homelabctl/internal/mdns"
	"github.com/jdillenberger/homelabctl/internal/scheduler"
)

var daemonMdnsCmd = &cobra.Command{
	Use:   "mdns",
	Short: "Run the mDNS advertiser",
	Long:  "Advertise the homelabctl instance and app routing domains via mDNS/Avahi.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if !cfg.MDNS.Enabled {
			return fmt.Errorf("mDNS is disabled in config")
		}

		runner := &exec.Runner{Verbose: verbose}
		mgr, err := newManager()
		if err != nil {
			return err
		}

		shutdown, err := startMDNS(cfg, mgr, runner, nil)
		if err != nil {
			return err
		}
		defer shutdown()

		fmt.Println("mDNS advertiser running")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		<-ctx.Done()
		fmt.Println("\nShutting down mDNS...")
		return nil
	},
}

// mdnsShutdownGroup collects cleanup functions for mDNS resources.
type mdnsShutdownGroup struct {
	funcs []func()
}

func (g *mdnsShutdownGroup) add(f func()) {
	g.funcs = append(g.funcs, f)
}

func (g *mdnsShutdownGroup) shutdown() {
	for i := len(g.funcs) - 1; i >= 0; i-- {
		g.funcs[i]()
	}
}

// startMDNS sets up mDNS service advertising and Avahi CNAME publishing.
// If sched is non-nil, periodic CNAME sync is registered as a scheduler job.
// If sched is nil (standalone mode), an internal ticker handles periodic sync.
// Returns a shutdown function and any startup error.
func startMDNS(cfg *config.Config, mgr *app.Manager, runner *exec.Runner, sched *scheduler.Scheduler) (func(), error) {
	var cleanup mdnsShutdownGroup

	// Advertise this homelabctl instance via mDNS
	deployedApps, err := mgr.ListDeployed()
	if err != nil {
		slog.Warn("Could not list deployed apps for mDNS", "error", err)
		deployedApps = nil
	}
	shutdownAdvertise, err := mdns.Advertise(cfg, version, deployedApps)
	if err != nil {
		return cleanup.shutdown, fmt.Errorf("mDNS advertising: %w", err)
	}
	cleanup.add(shutdownAdvertise)

	// Advertise app routing domains via Avahi CNAME
	if cfg.MDNS.AdvertiseApps {
		avahiMgr := mdns.NewAvahiCNAME(runner)
		avahiMgr.CleanStaleProcesses()
		cleanup.add(avahiMgr.Shutdown)

		var reconcileMu sync.Mutex
		reconcileCNAMEs := func() {
			reconcileMu.Lock()
			defer reconcileMu.Unlock()

			desired, err := mdns.DiscoverTraefikDomains(runner, cfg.Docker.Runtime)
			if err != nil {
				slog.Warn("Failed to discover Traefik domains", "error", err)
				return
			}

			published := avahiMgr.ListPublished()

			for domain := range published {
				if !desired[domain] {
					_ = avahiMgr.UnpublishCNAME(domain)
				}
			}

			for domain := range desired {
				if !published[domain] {
					if err := avahiMgr.PublishCNAME(domain, domain); err != nil {
						slog.Warn("Failed to publish CNAME", "domain", domain, "error", err)
					}
				}
			}
		}

		// Initial reconciliation
		reconcileCNAMEs()

		// Wire Manager callbacks for immediate reconciliation
		prevOnDeploy := mgr.OnDeploy
		mgr.OnDeploy = func(appName string, routing *app.DeployedRouting) {
			reconcileCNAMEs()
			if prevOnDeploy != nil {
				prevOnDeploy(appName, routing)
			}
		}
		prevOnRemove := mgr.OnRemove
		mgr.OnRemove = func(appName string) {
			reconcileCNAMEs()
			if prevOnRemove != nil {
				prevOnRemove(appName)
			}
		}

		// Register periodic reconciliation
		if sched != nil {
			if err := sched.Add(scheduler.Job{
				Name:     "mdns-cname-sync",
				Schedule: cfg.MDNS.Schedule,
				Func:     reconcileCNAMEs,
			}); err != nil {
				slog.Warn("mDNS CNAME sync scheduler failed to start", "error", err)
			}
		} else {
			// Standalone mode: use a ticker for periodic sync
			interval := parseDurationOrDefault(cfg.MDNS.Schedule, 30*time.Second)
			ticker := time.NewTicker(interval)
			cleanup.add(ticker.Stop)
			go func() {
				for range ticker.C {
					reconcileCNAMEs()
				}
			}()
		}
	}

	return cleanup.shutdown, nil
}

// parseDurationOrDefault tries to parse a duration string (e.g. "30s", "1m").
// If the string starts with "@every ", it strips the prefix.
// Falls back to the default on failure.
func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	// Handle cron-style "@every 30s"
	const prefix = "@every "
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		s = s[len(prefix):]
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CachedHealthResult holds a health result with a timestamp for TTL.
type CachedHealthResult struct {
	HealthResult
	CheckedAt time.Time `json:"checked_at"`
}

// HealthCache provides an in-memory cache of app health status,
// updated by a background goroutine polling docker compose ps.
type HealthCache struct {
	mu       sync.RWMutex
	results  map[string]CachedHealthResult
	interval time.Duration
	ttl      time.Duration
	compose  *Compose
	appsDir  string
	listFn   func() ([]string, error)

	cancel context.CancelFunc
	done   chan struct{}
}

// NewHealthCache creates a HealthCache. Call Start() to begin polling.
func NewHealthCache(compose *Compose, appsDir string, listFn func() ([]string, error), interval, ttl time.Duration) *HealthCache {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &HealthCache{
		results:  make(map[string]CachedHealthResult),
		interval: interval,
		ttl:      ttl,
		compose:  compose,
		appsDir:  appsDir,
		listFn:   listFn,
	}
}

// Start begins background polling. Runs one poll immediately so the
// first dashboard load already has data.
func (hc *HealthCache) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel
	hc.done = make(chan struct{})

	hc.poll()

	go func() {
		defer close(hc.done)
		ticker := time.NewTicker(hc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hc.poll()
			}
		}
	}()
	slog.Info("Health cache started", "interval", hc.interval, "ttl", hc.ttl)
}

// Stop halts background polling and waits for the goroutine to exit.
func (hc *HealthCache) Stop() {
	if hc.cancel != nil {
		hc.cancel()
		<-hc.done
	}
}

// Get returns the cached health status for an app.
// Returns HealthStatusUnknown if no result is cached or if it is stale.
func (hc *HealthCache) Get(appName string) CachedHealthResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	r, ok := hc.results[appName]
	if !ok || time.Since(r.CheckedAt) > hc.ttl {
		return CachedHealthResult{
			HealthResult: HealthResult{App: appName, Status: HealthStatusUnknown, Detail: "no cached result"},
		}
	}
	return r
}

// All returns cached results for all known apps.
func (hc *HealthCache) All() []CachedHealthResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	out := make([]CachedHealthResult, 0, len(hc.results))
	for _, r := range hc.results {
		if time.Since(r.CheckedAt) > hc.ttl {
			r.Status = HealthStatusUnknown
			r.Detail = "stale"
		}
		out = append(out, r)
	}
	return out
}

// poll runs a single health check cycle across all deployed apps.
func (hc *HealthCache) poll() {
	deployed, err := hc.listFn()
	if err != nil {
		slog.Error("Health cache: failed to list deployed apps", "error", err)
		return
	}

	now := time.Now()
	newResults := make(map[string]CachedHealthResult, len(deployed))

	for _, appName := range deployed {
		appDir := filepath.Join(hc.appsDir, appName)
		result := hc.checkApp(appName, appDir)
		result.CheckedAt = now
		newResults[appName] = result
	}

	hc.mu.Lock()
	hc.results = newResults
	hc.mu.Unlock()
}

// composeJSONContainer represents the relevant fields from docker compose ps --format json.
type composeJSONContainer struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

// checkApp runs docker compose ps --format json for one app and aggregates
// per-container health into a single app-level result.
func (hc *HealthCache) checkApp(appName, appDir string) CachedHealthResult {
	result, err := hc.compose.PSJson(appDir)
	if err != nil {
		return CachedHealthResult{
			HealthResult: HealthResult{App: appName, Status: HealthStatusUnknown, Detail: err.Error()},
		}
	}

	stdout := strings.TrimSpace(result.Stdout)
	if stdout == "" {
		return CachedHealthResult{
			HealthResult: HealthResult{App: appName, Status: HealthStatusUnhealthy, Detail: "no containers running"},
		}
	}

	var containers []composeJSONContainer
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c composeJSONContainer
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		containers = append(containers, c)
	}

	if len(containers) == 0 {
		return CachedHealthResult{
			HealthResult: HealthResult{App: appName, Status: HealthStatusUnhealthy, Detail: "no containers found"},
		}
	}

	aggregated := HealthStatusHealthy
	detail := fmt.Sprintf("%d container(s)", len(containers))
	hasAnyHealthcheck := false

	for _, c := range containers {
		if c.State != "running" {
			return CachedHealthResult{
				HealthResult: HealthResult{
					App:    appName,
					Status: HealthStatusUnhealthy,
					Detail: fmt.Sprintf("container %s is %s", c.Service, c.State),
				},
			}
		}

		switch c.Health {
		case "unhealthy":
			return CachedHealthResult{
				HealthResult: HealthResult{
					App:    appName,
					Status: HealthStatusUnhealthy,
					Detail: fmt.Sprintf("container %s is unhealthy", c.Service),
				},
			}
		case "starting":
			hasAnyHealthcheck = true
			if aggregated == HealthStatusHealthy {
				aggregated = HealthStatusStarting
				detail = fmt.Sprintf("container %s is starting", c.Service)
			}
		case "healthy":
			hasAnyHealthcheck = true
		case "":
			// No healthcheck defined for this container — treat as neutral.
		}
	}

	if !hasAnyHealthcheck {
		return CachedHealthResult{
			HealthResult: HealthResult{App: appName, Status: HealthStatusNone, Detail: "no healthcheck defined"},
		}
	}

	return CachedHealthResult{
		HealthResult: HealthResult{App: appName, Status: aggregated, Detail: detail},
	}
}

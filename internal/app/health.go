package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// HealthStatus represents the health state of an app.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusStarting  HealthStatus = "starting"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthResult holds the outcome of a health check.
type HealthResult struct {
	App    string       `json:"app"`
	Status HealthStatus `json:"status"`
	Detail string       `json:"detail,omitempty"`
}

// HealthChecker performs health checks on apps.
type HealthChecker struct {
	client *http.Client
}

// NewHealthChecker creates a new HealthChecker with a default timeout.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // health checks target localhost with self-signed certs
			},
		},
	}
}

// CheckHTTP performs an HTTP GET health check against the given URL.
func (hc *HealthChecker) CheckHTTP(ctx context.Context, url string) HealthResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return HealthResult{Status: HealthStatusUnhealthy, Detail: err.Error()}
	}
	resp, err := hc.client.Do(req)
	if err != nil {
		return HealthResult{Status: HealthStatusUnhealthy, Detail: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return HealthResult{Status: HealthStatusHealthy, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return HealthResult{Status: HealthStatusUnhealthy, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

// CheckTCP attempts a TCP connection to host:port.
func (hc *HealthChecker) CheckTCP(host string, port int) HealthResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return HealthResult{Status: HealthStatusUnhealthy, Detail: err.Error()}
	}
	conn.Close()
	return HealthResult{Status: HealthStatusHealthy, Detail: fmt.Sprintf("TCP %s reachable", addr)}
}

// CheckContainer checks if all containers in the project are running and healthy.
func (hc *HealthChecker) CheckContainer(compose *Compose, appDir string) HealthResult {
	result, err := compose.PS(appDir)
	if err != nil {
		return HealthResult{Status: HealthStatusUnknown, Detail: err.Error()}
	}
	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		return HealthResult{Status: HealthStatusUnhealthy, Detail: "no containers running"}
	}
	lines := strings.Split(output, "\n")
	// First line is the header row; check each subsequent line for status.
	for _, line := range lines[1:] {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "exit") || strings.Contains(lower, "dead") || strings.Contains(lower, "restarting") {
			return HealthResult{Status: HealthStatusUnhealthy, Detail: "one or more containers not running"}
		}
	}
	return HealthResult{Status: HealthStatusHealthy, Detail: fmt.Sprintf("%d container(s) running", len(lines)-1)}
}

// healthCheckTimeout returns the configured timeout for an app's health check,
// defaulting to 5 seconds.
func healthCheckTimeout(meta *AppMeta) time.Duration {
	if meta.HealthCheck != nil && meta.HealthCheck.Timeout != "" {
		if d, err := time.ParseDuration(meta.HealthCheck.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return 5 * time.Second
}

// CheckApp performs the appropriate health check for an app based on its metadata.
func (hc *HealthChecker) CheckApp(meta *AppMeta, compose *Compose, appDir string) HealthResult {
	result := HealthResult{App: meta.Name}

	// Try HTTP health check if configured
	if meta.HealthCheck != nil && meta.HealthCheck.URL != "" {
		timeout := healthCheckTimeout(meta)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		r := hc.CheckHTTP(ctx, meta.HealthCheck.URL)
		r.App = meta.Name
		return r
	}

	// Fall back to container check
	r := hc.CheckContainer(compose, appDir)
	r.App = meta.Name
	if r.Status == HealthStatusUnknown {
		result.Status = HealthStatusUnknown
		result.Detail = r.Detail
		return result
	}
	return r
}

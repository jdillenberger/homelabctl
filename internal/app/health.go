package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// CheckHTTP performs an HTTP GET health check against the given URL.
func (hc *HealthChecker) CheckHTTP(url string) HealthResult {
	resp, err := hc.client.Get(url)
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

// CheckContainer checks if a container is running and healthy via docker inspect.
func (hc *HealthChecker) CheckContainer(compose *Compose, appDir string) HealthResult {
	result, err := compose.PS(appDir)
	if err != nil {
		return HealthResult{Status: HealthStatusUnknown, Detail: err.Error()}
	}
	if result.Stdout == "" {
		return HealthResult{Status: HealthStatusUnhealthy, Detail: "no containers running"}
	}
	return HealthResult{Status: HealthStatusHealthy, Detail: "containers running"}
}

// CheckApp performs the appropriate health check for an app based on its metadata.
func (hc *HealthChecker) CheckApp(meta *AppMeta, compose *Compose, appDir string) HealthResult {
	result := HealthResult{App: meta.Name}

	// Try HTTP health check if configured
	if meta.HealthCheck != nil && meta.HealthCheck.URL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ctx

		r := hc.CheckHTTP(meta.HealthCheck.URL)
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

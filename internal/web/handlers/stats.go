package handlers

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemStats holds system statistics for display.
type SystemStats struct {
	CPUPercent  float64
	MemPercent  float64
	MemUsedGB   string
	MemTotalGB  string
	DiskPercent float64
	DiskUsedGB  string
	DiskTotalGB string
	Uptime      string
}

func collectStats() SystemStats {
	stats := SystemStats{}

	// CPU
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercents) > 0 {
		stats.CPUPercent = cpuPercents[0]
	}

	// Memory
	vmem, err := mem.VirtualMemory()
	if err == nil {
		stats.MemPercent = vmem.UsedPercent
		stats.MemUsedGB = fmt.Sprintf("%.1f", float64(vmem.Used)/(1024*1024*1024))
		stats.MemTotalGB = fmt.Sprintf("%.1f", float64(vmem.Total)/(1024*1024*1024))
	}

	// Disk
	diskStat, err := disk.Usage("/")
	if err == nil {
		stats.DiskPercent = diskStat.UsedPercent
		stats.DiskUsedGB = fmt.Sprintf("%.1f", float64(diskStat.Used)/(1024*1024*1024))
		stats.DiskTotalGB = fmt.Sprintf("%.1f", float64(diskStat.Total)/(1024*1024*1024))
	}

	// Uptime
	uptime, err := host.Uptime()
	if err == nil {
		days := uptime / 86400
		hours := (uptime % 86400) / 3600
		mins := (uptime % 3600) / 60
		if days > 0 {
			stats.Uptime = fmt.Sprintf("%dd %dh %dm", days, hours, mins)
		} else {
			stats.Uptime = fmt.Sprintf("%dh %dm", hours, mins)
		}
	} else {
		stats.Uptime = "N/A"
	}

	return stats
}

// StatsPartial renders just the stats panel HTML for htmx polling.
func (h *Handler) StatsPartial(c echo.Context) error {
	stats := collectStats()
	return c.Render(http.StatusOK, "stats_partial.html", stats)
}

// StatsCompact renders a compact one-line stats string for the nav bar.
func (h *Handler) StatsCompact(c echo.Context) error {
	stats := collectStats()
	return c.Render(http.StatusOK, "stats_compact.html", stats)
}

// statsJSON returns stats as a JSON-friendly map.
func statsJSON() map[string]interface{} {
	stats := collectStats()
	hostname, _ := os.Hostname()
	return map[string]interface{}{
		"hostname":      hostname,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"cpu_percent":   stats.CPUPercent,
		"mem_percent":   stats.MemPercent,
		"mem_used_gb":   stats.MemUsedGB,
		"mem_total_gb":  stats.MemTotalGB,
		"disk_percent":  stats.DiskPercent,
		"disk_used_gb":  stats.DiskUsedGB,
		"disk_total_gb": stats.DiskTotalGB,
		"uptime":        stats.Uptime,
	}
}

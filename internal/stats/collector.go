package stats

import (
	"fmt"
	"time"

	"context"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Stats holds system resource statistics.
type Stats struct {
	Timestamp   time.Time   `json:"timestamp"`
	CPU         CPUStats    `json:"cpu"`
	Memory      MemoryStats `json:"memory"`
	Disk        DiskStats   `json:"disk"`
	Temperature []TempStats `json:"temperature,omitempty"`
	Uptime      uint64      `json:"uptime_seconds"`
}

// CPUStats holds CPU usage information.
type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
}

// MemoryStats holds memory usage information.
type MemoryStats struct {
	Total       uint64  `json:"total_bytes"`
	Used        uint64  `json:"used_bytes"`
	Available   uint64  `json:"available_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// DiskStats holds disk usage information for the root partition.
type DiskStats struct {
	Total       uint64  `json:"total_bytes"`
	Used        uint64  `json:"used_bytes"`
	Free        uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Path        string  `json:"path"`
}

// TempStats holds temperature sensor data.
type TempStats struct {
	SensorKey   string  `json:"sensor_key"`
	Temperature float64 `json:"temperature_celsius"`
}

// Collect gathers current system statistics.
func Collect() (*Stats, error) {
	s := &Stats{
		Timestamp: time.Now(),
	}

	// CPU
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("reading cpu usage: %w", err)
	}
	if len(cpuPercent) > 0 {
		s.CPU.UsagePercent = cpuPercent[0]
	}
	cores, err := cpu.Counts(true)
	if err == nil {
		s.CPU.Cores = cores
	}

	// Memory
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("reading memory: %w", err)
	}
	s.Memory = MemoryStats{
		Total:       vm.Total,
		Used:        vm.Used,
		Available:   vm.Available,
		UsedPercent: vm.UsedPercent,
	}

	// Disk (root partition)
	du, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("reading disk usage: %w", err)
	}
	s.Disk = DiskStats{
		Total:       du.Total,
		Used:        du.Used,
		Free:        du.Free,
		UsedPercent: du.UsedPercent,
		Path:        "/",
	}

	// Temperature (best-effort, don't fail if unavailable)
	temps, err := sensors.TemperaturesWithContext(context.Background())
	if err == nil {
		for _, t := range temps {
			if t.Temperature > 0 {
				s.Temperature = append(s.Temperature, TempStats{
					SensorKey:   t.SensorKey,
					Temperature: t.Temperature,
				})
			}
		}
	}

	// Uptime
	uptime, err := host.Uptime()
	if err == nil {
		s.Uptime = uptime
	}

	return s, nil
}

// FormatBytes formats bytes into a human-readable string.
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

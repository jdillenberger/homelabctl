package stats

import (
	"testing"
)

func TestCollect(t *testing.T) {
	s, err := Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	t.Run("returns non-nil stats", func(t *testing.T) {
		if s == nil {
			t.Fatal("Collect() returned nil")
		}
	})

	t.Run("timestamp is set", func(t *testing.T) {
		if s.Timestamp.IsZero() {
			t.Error("Timestamp is zero")
		}
	})

	t.Run("CPU cores positive", func(t *testing.T) {
		if s.CPU.Cores <= 0 {
			t.Errorf("CPU.Cores = %d, expected > 0", s.CPU.Cores)
		}
	})

	t.Run("CPU usage in range", func(t *testing.T) {
		if s.CPU.UsagePercent < 0 || s.CPU.UsagePercent > 100 {
			t.Errorf("CPU.UsagePercent = %f, expected 0-100", s.CPU.UsagePercent)
		}
	})

	t.Run("memory total positive", func(t *testing.T) {
		if s.Memory.Total == 0 {
			t.Error("Memory.Total is 0")
		}
	})

	t.Run("memory used within total", func(t *testing.T) {
		if s.Memory.Used > s.Memory.Total {
			t.Errorf("Memory.Used (%d) > Memory.Total (%d)", s.Memory.Used, s.Memory.Total)
		}
	})

	t.Run("disk total positive", func(t *testing.T) {
		if s.Disk.Total == 0 {
			t.Error("Disk.Total is 0")
		}
	})

	t.Run("disk path is root", func(t *testing.T) {
		if s.Disk.Path != "/" {
			t.Errorf("Disk.Path = %q, expected /", s.Disk.Path)
		}
	})
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  uint64
		expect string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"one KiB", 1024, "1.0 KiB"},
		{"one MiB", 1048576, "1.0 MiB"},
		{"one GiB", 1073741824, "1.0 GiB"},
		{"one TiB", 1099511627776, "1.0 TiB"},
		{"mixed KiB", 1536, "1.5 KiB"},
		{"mixed GiB", 2684354560, "2.5 GiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.input)
			if got != tt.expect {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

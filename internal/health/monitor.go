package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jdillenberger/homelabctl/internal/app"
)

const historyFile = "health-history.json"

// CheckRecord holds the results of a single health check run.
type CheckRecord struct {
	Timestamp time.Time          `json:"timestamp"`
	Results   []app.HealthResult `json:"results"`
}

// Monitor records and retrieves health check history.
type Monitor struct {
	dataDir    string
	maxRecords int
}

// NewMonitor creates a new Monitor.
func NewMonitor(dataDir string, maxRecords int) *Monitor {
	if maxRecords <= 0 {
		maxRecords = 1000
	}
	return &Monitor{
		dataDir:    dataDir,
		maxRecords: maxRecords,
	}
}

// Record appends a health check result and trims old entries.
func (m *Monitor) Record(results []app.HealthResult) error {
	history, err := m.LoadHistory()
	if err != nil {
		history = nil
	}

	history = append(history, CheckRecord{
		Timestamp: time.Now(),
		Results:   results,
	})

	if len(history) > m.maxRecords {
		history = history[len(history)-m.maxRecords:]
	}

	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	data, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("marshaling health history: %w", err)
	}

	path := filepath.Join(m.dataDir, historyFile)
	return os.WriteFile(path, data, 0o644)
}

// LoadHistory reads health check history from disk.
func (m *Monitor) LoadHistory() ([]CheckRecord, error) {
	path := filepath.Join(m.dataDir, historyFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading health history: %w", err)
	}

	var history []CheckRecord
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("parsing health history: %w", err)
	}
	return history, nil
}

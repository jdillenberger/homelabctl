package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HostState tracks per-host state information.
type HostState struct {
	Hostname     string               `json:"hostname"`
	LastSeen     time.Time            `json:"last_seen"`
	DeployedApps map[string]AppRecord `json:"deployed_apps"`
}

// AppRecord tracks a deployed app on this host.
type AppRecord struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	DeployedAt time.Time `json:"deployed_at"`
	LastSeen   time.Time `json:"last_seen"`
}

const hostStateFile = "host-state.json"

// LoadHostState reads the host state from the data directory.
func LoadHostState(dataDir string) (*HostState, error) {
	path := filepath.Join(dataDir, hostStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			hostname, _ := os.Hostname()
			return &HostState{
				Hostname:     hostname,
				LastSeen:     time.Now(),
				DeployedApps: make(map[string]AppRecord),
			}, nil
		}
		return nil, fmt.Errorf("reading host state: %w", err)
	}

	var state HostState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing host state: %w", err)
	}
	if state.DeployedApps == nil {
		state.DeployedApps = make(map[string]AppRecord)
	}
	return &state, nil
}

// Save writes the host state to the data directory.
func (h *HostState) Save(dataDir string) error {
	h.LastSeen = time.Now()

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling host state: %w", err)
	}

	path := filepath.Join(dataDir, hostStateFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing host state: %w", err)
	}
	return nil
}

// RecordApp adds or updates an app record in the host state.
func (h *HostState) RecordApp(name, version string, deployedAt time.Time) {
	h.DeployedApps[name] = AppRecord{
		Name:       name,
		Version:    version,
		DeployedAt: deployedAt,
		LastSeen:   time.Now(),
	}
}

// RemoveApp removes an app record from the host state.
func (h *HostState) RemoveApp(name string) {
	delete(h.DeployedApps, name)
}

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultFleetConfigPath = "/etc/homelabctl/fleet.yaml"

// FleetConfig holds the fleet-wide configuration.
type FleetConfig struct {
	Fleet    FleetMeta     `yaml:"fleet"`
	Hosts    []FleetHost   `yaml:"hosts"`
	Defaults FleetDefaults `yaml:"defaults"`
}

// FleetMeta holds fleet identification and authentication.
type FleetMeta struct {
	Name   string `yaml:"name"`
	Secret string `yaml:"secret"` // PSK for API auth
}

// FleetHost represents a single host in the fleet.
type FleetHost struct {
	Hostname string   `yaml:"hostname"`
	Role     string   `yaml:"role"` // primary, worker
	Apps     []string `yaml:"apps"`
	Address  string   `yaml:"address"` // discovered or configured IP
	Port     int      `yaml:"port"`
	Online   bool     `yaml:"-"`
	Version  string   `yaml:"-"`
}

// FleetDefaults holds default settings applied to all hosts.
type FleetDefaults struct {
	DomainSuffix string       `yaml:"domain_suffix"`
	Backup       BackupConfig `yaml:"backup"`
}

// LoadFleetConfig reads the fleet configuration from disk.
func LoadFleetConfig() (*FleetConfig, error) {
	path := defaultFleetConfigPath

	// Allow override via environment variable
	if envPath := os.Getenv("HOMELABCTL_FLEET_CONFIG"); envPath != "" {
		path = envPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultFleetConfig(), nil
		}
		return nil, fmt.Errorf("reading fleet config: %w", err)
	}

	var cfg FleetConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing fleet config: %w", err)
	}

	// Apply defaults for missing ports
	for i := range cfg.Hosts {
		if cfg.Hosts[i].Port == 0 {
			cfg.Hosts[i].Port = 8080
		}
	}

	return &cfg, nil
}

// SaveFleetConfig writes the fleet configuration to disk.
func SaveFleetConfig(cfg *FleetConfig) error {
	path := defaultFleetConfigPath

	if envPath := os.Getenv("HOMELABCTL_FLEET_CONFIG"); envPath != "" {
		path = envPath
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling fleet config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing fleet config: %w", err)
	}

	return nil
}

func defaultFleetConfig() *FleetConfig {
	hostname, _ := os.Hostname()
	return &FleetConfig{
		Fleet: FleetMeta{
			Name: "homelab",
		},
		Hosts: []FleetHost{
			{
				Hostname: hostname,
				Role:     "primary",
				Port:     8080,
			},
		},
		Defaults: FleetDefaults{
			DomainSuffix: "local",
		},
	}
}

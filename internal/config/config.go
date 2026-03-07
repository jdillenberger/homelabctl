package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// DefaultTemplatesDir returns the default local templates directory.
func DefaultTemplatesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".homelabctl", "templates")
}

// Config holds the global homelabctl configuration.
type Config struct {
	Hostname     string `mapstructure:"hostname" yaml:"hostname"`
	AppsDir      string `mapstructure:"apps_dir" yaml:"apps_dir"`
	DataDir      string `mapstructure:"data_dir" yaml:"data_dir"`
	TemplatesDir string `mapstructure:"templates_dir" yaml:"templates_dir"`

	Network NetworkConfig `mapstructure:"network" yaml:"network"`
	Web     WebConfig     `mapstructure:"web" yaml:"web"`
	Docker  DockerConfig  `mapstructure:"docker" yaml:"docker"`
	MDNS    MDNSConfig    `mapstructure:"mdns" yaml:"mdns"`
	Backup  BackupConfig  `mapstructure:"backup" yaml:"backup"`
	Alerts  AlertsConfig  `mapstructure:"alerts" yaml:"alerts"`
	Health  HealthConfig  `mapstructure:"health" yaml:"health"`
	Updates UpdatesConfig `mapstructure:"updates" yaml:"updates"`
	Prune   PruneConfig   `mapstructure:"prune" yaml:"prune"`

	Routing RoutingConfig `mapstructure:"routing" yaml:"routing"`

	Integrations IntegrationsConfig `mapstructure:"integrations" yaml:"integrations"`
}

type RoutingConfig struct {
	Enabled  bool        `mapstructure:"enabled" yaml:"enabled"`
	Provider string      `mapstructure:"provider" yaml:"provider"`
	Domain   string      `mapstructure:"domain" yaml:"domain"`
	HTTPS    HTTPSConfig `mapstructure:"https" yaml:"https"`
}

type HTTPSConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	AcmeEmail string `mapstructure:"acme_email" yaml:"acme_email"`
}

type NetworkConfig struct {
	Domain  string `mapstructure:"domain" yaml:"domain"`
	WebPort int    `mapstructure:"web_port" yaml:"web_port"`
}

type WebConfig struct {
	NavColor string `mapstructure:"nav_color" yaml:"nav_color"`
}

type DockerConfig struct {
	Runtime        string `mapstructure:"runtime" yaml:"runtime"`
	ComposeCommand string `mapstructure:"compose_command" yaml:"compose_command"`
	DefaultNetwork string `mapstructure:"default_network" yaml:"default_network"`
}

type MDNSConfig struct {
	Enabled       bool `mapstructure:"enabled" yaml:"enabled"`
	AdvertiseApps bool `mapstructure:"advertise_apps" yaml:"advertise_apps"`
}

type BackupConfig struct {
	Enabled   bool            `mapstructure:"enabled" yaml:"enabled"`
	BorgRepo  string          `mapstructure:"borg_repo" yaml:"borg_repo"`
	Schedule  string          `mapstructure:"schedule" yaml:"schedule"`
	Retention RetentionConfig `mapstructure:"retention" yaml:"retention"`
}

type RetentionConfig struct {
	KeepDaily   int `mapstructure:"keep_daily" yaml:"keep_daily"`
	KeepWeekly  int `mapstructure:"keep_weekly" yaml:"keep_weekly"`
	KeepMonthly int `mapstructure:"keep_monthly" yaml:"keep_monthly"`
}

type AlertsConfig struct {
	Enabled  bool                `mapstructure:"enabled" yaml:"enabled"`
	Schedule string              `mapstructure:"schedule" yaml:"schedule"`
	Cooldown string              `mapstructure:"cooldown" yaml:"cooldown"`
	Channels AlertChannelsConfig `mapstructure:"channels" yaml:"channels"`
}

type AlertChannelsConfig struct {
	Webhook *WebhookChannelConfig `mapstructure:"webhook" yaml:"webhook,omitempty"`
	Ntfy    *NtfyChannelConfig    `mapstructure:"ntfy" yaml:"ntfy,omitempty"`
}

type WebhookChannelConfig struct {
	URL string `mapstructure:"url" yaml:"url"`
}

type NtfyChannelConfig struct {
	URL   string `mapstructure:"url" yaml:"url"`
	Token string `mapstructure:"token" yaml:"token,omitempty"`
}

type HealthConfig struct {
	Enabled    bool   `mapstructure:"enabled" yaml:"enabled"`
	Schedule   string `mapstructure:"schedule" yaml:"schedule"`
	MaxHistory int    `mapstructure:"max_history" yaml:"max_history"`
}

type UpdatesConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	Schedule string `mapstructure:"schedule" yaml:"schedule"`
}

type PruneConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	Schedule string `mapstructure:"schedule" yaml:"schedule"`
}

type IntegrationsConfig struct {
	Keycloak          KeycloakConfig `mapstructure:"keycloak" yaml:"keycloak"`
	Beszel            BeszelConfig   `mapstructure:"beszel" yaml:"beszel"`
	NginxProxyManager NPMConfig      `mapstructure:"nginx_proxy_manager" yaml:"nginx_proxy_manager"`
}

type KeycloakConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	URL     string `mapstructure:"url" yaml:"url"`
	Realm   string `mapstructure:"realm" yaml:"realm"`
}

type BeszelConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	HubURL  string `mapstructure:"hub_url" yaml:"hub_url"`
}

type NPMConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	URL     string `mapstructure:"url" yaml:"url"`
}

// DefaultConfig returns the configuration with sensible defaults.
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		Hostname:     hostname,
		AppsDir:      "/opt/homelabctl/apps",
		DataDir:      "/opt/homelabctl/data",
		TemplatesDir: DefaultTemplatesDir(),
		Network: NetworkConfig{
			Domain:  "local",
			WebPort: 8080,
		},
		Docker: DockerConfig{
			Runtime:        "docker",
			ComposeCommand: "docker compose",
			DefaultNetwork: "homelabctl-net",
		},
		MDNS: MDNSConfig{
			Enabled:       true,
			AdvertiseApps: true,
		},
		Backup: BackupConfig{
			Enabled:  false,
			BorgRepo: "/mnt/backup/borg",
			Schedule: "0 3 * * *",
			Retention: RetentionConfig{
				KeepDaily:   7,
				KeepWeekly:  4,
				KeepMonthly: 6,
			},
		},
		Alerts: AlertsConfig{
			Enabled:  false,
			Schedule: "*/5 * * * *",
			Cooldown: "15m",
		},
		Health: HealthConfig{
			Enabled:    false,
			Schedule:   "*/2 * * * *",
			MaxHistory: 1000,
		},
		Updates: UpdatesConfig{
			Enabled:  false,
			Schedule: "0 4 * * 0",
		},
		Prune: PruneConfig{
			Enabled:  false,
			Schedule: "0 5 * * 0",
		},
		Routing: RoutingConfig{
			Enabled:  false,
			Provider: "traefik",
			Domain:   "",
			HTTPS: HTTPSConfig{
				Enabled: true,
			},
		},
	}
}

// SetDefaults configures viper defaults.
func SetDefaults() {
	d := DefaultConfig()
	viper.SetDefault("hostname", d.Hostname)
	viper.SetDefault("apps_dir", d.AppsDir)
	viper.SetDefault("data_dir", d.DataDir)
	viper.SetDefault("templates_dir", d.TemplatesDir)
	viper.SetDefault("network.domain", d.Network.Domain)
	viper.SetDefault("network.web_port", d.Network.WebPort)
	viper.SetDefault("docker.runtime", d.Docker.Runtime)
	viper.SetDefault("docker.compose_command", d.Docker.ComposeCommand)
	viper.SetDefault("docker.default_network", d.Docker.DefaultNetwork)
	viper.SetDefault("mdns.enabled", d.MDNS.Enabled)
	viper.SetDefault("mdns.advertise_apps", d.MDNS.AdvertiseApps)
	viper.SetDefault("backup.enabled", d.Backup.Enabled)
	viper.SetDefault("backup.borg_repo", d.Backup.BorgRepo)
	viper.SetDefault("backup.schedule", d.Backup.Schedule)
	viper.SetDefault("backup.retention.keep_daily", d.Backup.Retention.KeepDaily)
	viper.SetDefault("backup.retention.keep_weekly", d.Backup.Retention.KeepWeekly)
	viper.SetDefault("backup.retention.keep_monthly", d.Backup.Retention.KeepMonthly)
	viper.SetDefault("alerts.enabled", d.Alerts.Enabled)
	viper.SetDefault("alerts.schedule", d.Alerts.Schedule)
	viper.SetDefault("alerts.cooldown", d.Alerts.Cooldown)
	viper.SetDefault("health.enabled", d.Health.Enabled)
	viper.SetDefault("health.schedule", d.Health.Schedule)
	viper.SetDefault("health.max_history", d.Health.MaxHistory)
	viper.SetDefault("updates.enabled", d.Updates.Enabled)
	viper.SetDefault("updates.schedule", d.Updates.Schedule)
	viper.SetDefault("prune.enabled", d.Prune.Enabled)
	viper.SetDefault("prune.schedule", d.Prune.Schedule)
	viper.SetDefault("routing.enabled", d.Routing.Enabled)
	viper.SetDefault("routing.provider", d.Routing.Provider)
	viper.SetDefault("routing.domain", d.Routing.Domain)
	viper.SetDefault("routing.https.enabled", d.Routing.HTTPS.Enabled)
	viper.SetDefault("routing.https.acme_email", d.Routing.HTTPS.AcmeEmail)
}

// Load reads the global config from viper into a Config struct.
func Load() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// EnsureDirectories creates the apps and data directories if they don't exist.
func (c *Config) EnsureDirectories() error {
	for _, dir := range []string{c.AppsDir, c.DataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}
	return nil
}

// Validate checks the config for common errors and returns a list of issues.
func Validate(c *Config) []string {
	var errs []string

	if c.Hostname == "" {
		errs = append(errs, "hostname is empty")
	}
	if c.AppsDir == "" {
		errs = append(errs, "apps_dir is empty")
	}
	if c.DataDir == "" {
		errs = append(errs, "data_dir is empty")
	}
	if c.Network.Domain == "" {
		errs = append(errs, "network.domain is empty")
	}
	if c.Network.WebPort < 1 || c.Network.WebPort > 65535 {
		errs = append(errs, "network.web_port must be between 1 and 65535")
	}
	if c.Docker.Runtime == "" {
		errs = append(errs, "docker.runtime is empty")
	}
	if c.Docker.ComposeCommand == "" {
		errs = append(errs, "docker.compose_command is empty")
	}
	if c.Docker.DefaultNetwork == "" {
		errs = append(errs, "docker.default_network is empty")
	}
	if c.Backup.Enabled && c.Backup.BorgRepo == "" {
		errs = append(errs, "backup.borg_repo is empty but backup is enabled")
	}
	if c.Routing.Enabled {
		if c.Routing.Provider != "traefik" {
			errs = append(errs, "routing.provider must be \"traefik\"")
		}
	}

	return errs
}

// ReposDir returns the directory where template repos are cloned.
func (c *Config) ReposDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".homelabctl", "repos")
}

// ManifestPath returns the path to the repos manifest file.
func (c *Config) ManifestPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".homelabctl", "repos.yaml")
}

// RoutingDomain returns the effective routing domain, falling back to network.domain.
func (c *Config) RoutingDomain() string {
	if c.Routing.Domain != "" {
		return c.Routing.Domain
	}
	return c.Hostname + "." + c.Network.Domain
}

// AppDir returns the directory for a specific deployed app.
func (c *Config) AppDir(appName string) string {
	return filepath.Join(c.AppsDir, appName)
}

// DataPath returns the data directory for a specific app.
func (c *Config) DataPath(appName string) string {
	return filepath.Join(c.DataDir, appName)
}

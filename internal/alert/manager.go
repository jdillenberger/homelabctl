package alert

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// SystemStats holds the stats values needed for rule evaluation.
type SystemStats struct {
	CPUPercent  float64
	MemPercent  float64
	MemUsedGB   string
	MemTotalGB  string
	DiskPercent float64
	DiskUsedGB  string
	DiskTotalGB string
}

// Manager evaluates alert rules and dispatches notifications.
type Manager struct {
	store            *Store
	notifiers        map[string]Notifier
	cooldowns        map[string]time.Time
	cooldownDuration time.Duration
	mu               sync.Mutex
}

// NewManager creates a new alert Manager.
func NewManager(store *Store, cooldown time.Duration) *Manager {
	return &Manager{
		store:            store,
		notifiers:        make(map[string]Notifier),
		cooldowns:        make(map[string]time.Time),
		cooldownDuration: cooldown,
	}
}

// RegisterNotifiers sets up notifiers from channel configuration.
func (m *Manager) RegisterNotifiers(channels config.AlertChannelsConfig) {
	if channels.Webhook != nil && channels.Webhook.URL != "" {
		m.notifiers["webhook"] = NewWebhookNotifier(channels.Webhook.URL)
	}
	if channels.Ntfy != nil && channels.Ntfy.URL != "" {
		m.notifiers["ntfy"] = NewNtfyNotifier(channels.Ntfy.URL, channels.Ntfy.Token)
	}
	if channels.Gotify != nil && channels.Gotify.URL != "" {
		m.notifiers["gotify"] = NewGotifyNotifier(channels.Gotify.URL, channels.Gotify.Token)
	}
	if channels.Email != nil && channels.Email.Host != "" {
		m.notifiers["email"] = NewEmailNotifier(
			channels.Email.Host, channels.Email.Port,
			channels.Email.From, channels.Email.To,
			channels.Email.Username, channels.Email.Password,
		)
	}
}

// Store returns the underlying alert store.
func (m *Manager) Store() *Store {
	return m.store
}

// Evaluate checks rules against current stats and health results, firing alerts as needed.
func (m *Manager) Evaluate(stats *SystemStats, healthResults []app.HealthResult) {
	rules, err := m.store.LoadRules()
	if err != nil {
		slog.Error("Failed to load alert rules", "error", err)
		return
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		m.evaluateRule(rule, stats, healthResults)
	}
}

func (m *Manager) evaluateRule(rule Rule, stats *SystemStats, healthResults []app.HealthResult) {
	var fired bool
	var message, detail string

	switch rule.Type {
	case RuleTypeDiskFull:
		if stats != nil && stats.DiskPercent >= rule.Threshold {
			fired = true
			message = fmt.Sprintf("Disk usage at %.1f%% (threshold: %.0f%%)", stats.DiskPercent, rule.Threshold)
			detail = fmt.Sprintf("Used: %s / %s GB", stats.DiskUsedGB, stats.DiskTotalGB)
		}
	case RuleTypeHighCPU:
		if stats != nil && stats.CPUPercent >= rule.Threshold {
			fired = true
			message = fmt.Sprintf("CPU usage at %.1f%% (threshold: %.0f%%)", stats.CPUPercent, rule.Threshold)
		}
	case RuleTypeHighMemory:
		if stats != nil && stats.MemPercent >= rule.Threshold {
			fired = true
			message = fmt.Sprintf("Memory usage at %.1f%% (threshold: %.0f%%)", stats.MemPercent, rule.Threshold)
			detail = fmt.Sprintf("Used: %s / %s GB", stats.MemUsedGB, stats.MemTotalGB)
		}
	case RuleTypeAppDown:
		for _, hr := range healthResults {
			if rule.App != "" && hr.App != rule.App {
				continue
			}
			if hr.Status == app.HealthStatusUnhealthy {
				fired = true
				message = fmt.Sprintf("App %s is down", hr.App)
				detail = hr.Detail
				break
			}
		}
	}

	if !fired {
		return
	}

	// Check cooldown
	m.mu.Lock()
	cooldownKey := fmt.Sprintf("%s:%s", rule.Type, rule.App)
	if lastFired, ok := m.cooldowns[cooldownKey]; ok {
		if time.Since(lastFired) < m.cooldownDuration {
			m.mu.Unlock()
			return
		}
	}
	m.cooldowns[cooldownKey] = time.Now()
	m.mu.Unlock()

	a := Alert{
		ID:        uuid.New().String(),
		Type:      string(rule.Type),
		Severity:  SeverityWarning,
		Message:   message,
		Detail:    detail,
		Timestamp: time.Now(),
	}

	if rule.Type == RuleTypeAppDown {
		a.Severity = SeverityCritical
	}

	m.dispatch(a, rule.Channels)
}

// NotifyBackupFailed sends a backup failure alert.
func (m *Manager) NotifyBackupFailed(appName string, err error) {
	a := Alert{
		ID:        uuid.New().String(),
		Type:      string(RuleTypeBackupFailed),
		Severity:  SeverityCritical,
		Message:   fmt.Sprintf("Backup failed for %s", appName),
		Detail:    err.Error(),
		Timestamp: time.Now(),
	}
	m.dispatchAll(a)
}

// NotifyUpdateFailed sends an update failure alert.
func (m *Manager) NotifyUpdateFailed(appName string, err error) {
	a := Alert{
		ID:        uuid.New().String(),
		Type:      "update-failed",
		Severity:  SeverityWarning,
		Message:   fmt.Sprintf("Auto-update failed for %s", appName),
		Detail:    err.Error(),
		Timestamp: time.Now(),
	}
	m.dispatchAll(a)
}

// SendTest sends a test alert to all configured notifiers.
func (m *Manager) SendTest() error {
	a := Alert{
		ID:        uuid.New().String(),
		Type:      "test",
		Severity:  SeverityInfo,
		Message:   "Test alert from homelabctl",
		Detail:    "This is a test notification to verify your alert channels are working.",
		Timestamp: time.Now(),
	}
	return m.dispatchAll(a)
}

func (m *Manager) dispatch(a Alert, channels []string) {
	if err := m.store.AppendHistory(a); err != nil {
		slog.Error("Failed to save alert to history", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, ch := range channels {
		notifier, ok := m.notifiers[ch]
		if !ok {
			slog.Warn("Unknown alert channel", "channel", ch)
			continue
		}
		if err := notifier.Send(ctx, a); err != nil {
			slog.Error("Failed to send alert", "channel", ch, "error", err)
		}
	}
}

func (m *Manager) dispatchAll(a Alert) error {
	if err := m.store.AppendHistory(a); err != nil {
		slog.Error("Failed to save alert to history", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for _, notifier := range m.notifiers {
		if err := notifier.Send(ctx, a); err != nil {
			slog.Error("Failed to send alert", "channel", notifier.Name(), "error", err)
			lastErr = err
		}
	}
	return lastErr
}

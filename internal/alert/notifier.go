package alert

import (
	"context"
	"time"
)

// Severity represents the alert severity level.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Alert represents an alert event.
type Alert struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Severity  Severity  `json:"severity"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Resolved  bool      `json:"resolved"`
}

// Notifier is the interface for sending alert notifications.
type Notifier interface {
	Name() string
	Send(ctx context.Context, alert Alert) error
}

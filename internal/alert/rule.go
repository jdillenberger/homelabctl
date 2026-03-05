package alert

// RuleType represents the type of alert rule.
type RuleType string

const (
	RuleTypeAppDown      RuleType = "app-down"
	RuleTypeBackupFailed RuleType = "backup-failed"
)

// ValidRuleTypes lists all valid rule types.
var ValidRuleTypes = []RuleType{
	RuleTypeAppDown,
	RuleTypeBackupFailed,
}

// Rule defines an alert rule that is evaluated periodically.
type Rule struct {
	ID        string   `yaml:"id" json:"id"`
	Type      RuleType `yaml:"type" json:"type"`
	Threshold float64  `yaml:"threshold" json:"threshold"`
	App       string   `yaml:"app,omitempty" json:"app,omitempty"`
	Channels  []string `yaml:"channels" json:"channels"`
	Enabled   bool     `yaml:"enabled" json:"enabled"`
}

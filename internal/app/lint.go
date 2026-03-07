package app

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LintSeverity represents the severity of a lint finding.
type LintSeverity string

const (
	SeverityError   LintSeverity = "ERROR"
	SeverityWarning LintSeverity = "WARNING"
	SeverityInfo    LintSeverity = "INFO"
)

// LintFinding represents a single lint finding.
type LintFinding struct {
	Template string       `json:"template"`
	File     string       `json:"file"`
	Severity LintSeverity `json:"severity"`
	Check    string       `json:"check"`
	Message  string       `json:"message"`
}

// LintResult holds all findings from a lint run.
type LintResult struct {
	Findings []LintFinding `json:"findings"`
	Summary  LintSummary   `json:"summary"`
}

// LintSummary counts findings by severity.
type LintSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
	Total    int `json:"total"`
}

// Linter inspects templates for common issues.
type Linter struct {
	registry *Registry
}

// NewLinter creates a new Linter backed by the given registry.
func NewLinter(registry *Registry) *Linter {
	return &Linter{registry: registry}
}

// --- Compose YAML structs (minimal, just the fields we check) ---

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	Networks map[string]composeNetwork `yaml:"networks"`
}

type composeService struct {
	Image         string            `yaml:"image"`
	Build         interface{}       `yaml:"build"`
	Restart       string            `yaml:"restart"`
	ContainerName string            `yaml:"container_name"`
	SecurityOpt   []string          `yaml:"security_opt"`
	Logging       *composeLogging   `yaml:"logging"`
	Healthcheck   *composeHC        `yaml:"healthcheck"`
	MemLimit      string            `yaml:"mem_limit"`
	PidsLimit     interface{}       `yaml:"pids_limit"`
	Deploy        *composeDeploy    `yaml:"deploy"`
	CapDrop       []string          `yaml:"cap_drop"`
	ReadOnly      *bool             `yaml:"read_only"`
}

type composeLogging struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options"`
}

type composeHC struct {
	Test interface{} `yaml:"test"`
}

type composeDeploy struct {
	Resources *composeResources `yaml:"resources"`
}

type composeResources struct {
	Limits *composeResourceLimits `yaml:"limits"`
}

type composeResourceLimits struct {
	Memory string      `yaml:"memory"`
	Pids   interface{} `yaml:"pids"`
}

type composeNetwork struct {
	External interface{} `yaml:"external"`
}

// --- Standard values injected at deploy time ---

var standardValues = map[string]string{
	"hostname": "lint-host",
	"domain":   "example.com",
	"data_dir": "/data/lint-app",
	"app_name": "lint-app",
	"network":  "homelabctl",
	"timezone": "UTC",
}

// LintAll lints all templates and returns a combined result.
func (l *Linter) LintAll() *LintResult {
	var allFindings []LintFinding
	for _, name := range l.registry.List() {
		findings := l.LintTemplate(name)
		allFindings = append(allFindings, findings...)
	}
	return buildResult(allFindings)
}

// LintTemplate lints a single template and returns its findings.
func (l *Linter) LintTemplate(name string) []LintFinding {
	meta, ok := l.registry.Get(name)
	if !ok {
		return []LintFinding{{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityError,
			Check:    "template-not-found",
			Message:  "Template not found in registry",
		}}
	}

	var findings []LintFinding

	// Lint app.yaml
	findings = append(findings, l.lintAppYAML(name, meta)...)

	// Build dummy values for template rendering
	dummyValues := l.buildDummyValues(meta)

	// Lint template rendering and compose
	findings = append(findings, l.lintCompose(name, meta, dummyValues)...)

	// Lint value usage
	findings = append(findings, l.lintValueUsage(name, meta)...)

	// Filter out suppressed findings
	if len(meta.LintIgnore) > 0 {
		findings = filterSuppressed(findings, meta.LintIgnore)
	}

	return findings
}

// filterSuppressed removes findings whose Check matches a lint_ignore entry.
// Entries can be exact check IDs ("floating-image-tag") or qualified
// with a suffix ("secret-no-autogen:password_hash") to suppress only
// when the message contains that suffix.
func filterSuppressed(findings []LintFinding, ignores []string) []LintFinding {
	var kept []LintFinding
	for _, f := range findings {
		if isSuppressed(f, ignores) {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

func isSuppressed(f LintFinding, ignores []string) bool {
	for _, ig := range ignores {
		if parts := strings.SplitN(ig, ":", 2); len(parts) == 2 {
			// Qualified: check ID + qualifier must appear in message
			if f.Check == parts[0] && strings.Contains(f.Message, parts[1]) {
				return true
			}
		} else {
			// Exact check ID match
			if f.Check == ig {
				return true
			}
		}
	}
	return false
}

func (l *Linter) lintAppYAML(name string, meta *AppMeta) []LintFinding {
	var findings []LintFinding

	if meta.Description == "" || strings.HasPrefix(meta.Description, "TODO") {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityWarning,
			Check:    "description-missing",
			Message:  "Description is empty or starts with TODO",
		})
	}

	if meta.Version == "" {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityWarning,
			Check:    "version-missing",
			Message:  "No version specified",
		})
	}

	if meta.HealthCheck == nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityWarning,
			Check:    "healthcheck-missing",
			Message:  "No health_check defined",
		})
	}

	if meta.Backup == nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityInfo,
			Check:    "backup-missing",
			Message:  "No backup section defined",
		})
	}

	if meta.PostDeployInfo == nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityInfo,
			Check:    "post-deploy-info-missing",
			Message:  "No post_deploy_info section defined",
		})
	}

	if meta.Requirements == nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "app.yaml",
			Severity: SeverityInfo,
			Check:    "requirements-missing",
			Message:  "No requirements section defined",
		})
	}

	// Check for secrets without auto_gen or default
	for _, v := range meta.Values {
		if v.Secret && v.AutoGen == "" && v.Default == "" {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "app.yaml",
				Severity: SeverityWarning,
				Check:    "secret-no-autogen",
				Message:  fmt.Sprintf("Secret value %q has no auto_gen or default", v.Name),
			})
		}
	}

	return findings
}

func (l *Linter) buildDummyValues(meta *AppMeta) map[string]string {
	values := make(map[string]string)

	// Start with standard values
	for k, v := range standardValues {
		values[k] = v
	}

	// Override app_name with actual template name
	values["app_name"] = meta.Name

	// Add values from app.yaml
	for _, v := range meta.Values {
		switch {
		case v.Default != "":
			values[v.Name] = v.Default
		case v.Secret:
			values[v.Name] = "lint-secret-placeholder"
		default:
			values[v.Name] = "lint-placeholder"
		}
	}

	return values
}

func (l *Linter) lintCompose(name string, meta *AppMeta, dummyValues map[string]string) []LintFinding {
	var findings []LintFinding

	// Try to render the compose template
	renderer := NewTemplateRenderer(l.registry)
	rendered, err := renderer.RenderFile(name, "docker-compose.yml.tmpl", dummyValues)
	if err != nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "docker-compose.yml.tmpl",
			Severity: SeverityError,
			Check:    "template-render-error",
			Message:  fmt.Sprintf("Template failed to render: %v", err),
		})
		return findings
	}

	// Parse the rendered compose YAML
	var compose composeFile
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		findings = append(findings, LintFinding{
			Template: name,
			File:     "docker-compose.yml.tmpl",
			Severity: SeverityError,
			Check:    "template-render-error",
			Message:  fmt.Sprintf("Rendered compose YAML is invalid: %v", err),
		})
		return findings
	}

	// Check each service
	for svcName, svc := range compose.Services {
		prefix := fmt.Sprintf("services.%s", svcName)

		// missing-restart
		if svc.Restart == "" {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityError,
				Check:    "missing-restart",
				Message:  fmt.Sprintf("%s: missing restart policy", prefix),
			})
		}

		// missing-container-name
		if svc.ContainerName == "" {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-container-name",
				Message:  fmt.Sprintf("%s: missing container_name", prefix),
			})
		}

		// missing-no-new-privileges
		hasNoNewPriv := false
		for _, opt := range svc.SecurityOpt {
			if strings.Contains(opt, "no-new-privileges") {
				hasNoNewPriv = true
				break
			}
		}
		if !hasNoNewPriv {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-no-new-privileges",
				Message:  fmt.Sprintf("%s: missing security_opt: no-new-privileges:true", prefix),
			})
		}

		// missing-logging
		if svc.Logging == nil {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-logging",
				Message:  fmt.Sprintf("%s: no logging configuration", prefix),
			})
		}

		// missing-healthcheck
		if svc.Healthcheck == nil {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-healthcheck",
				Message:  fmt.Sprintf("%s: no healthcheck defined", prefix),
			})
		}

		// missing-memory-limit
		hasMemLimit := svc.MemLimit != ""
		if svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil {
			if svc.Deploy.Resources.Limits.Memory != "" {
				hasMemLimit = true
			}
		}
		if !hasMemLimit {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-memory-limit",
				Message:  fmt.Sprintf("%s: no memory limit (mem_limit or deploy.resources.limits.memory)", prefix),
			})
		}

		// missing-pids-limit
		hasPidsLimit := svc.PidsLimit != nil
		if svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil {
			if svc.Deploy.Resources.Limits.Pids != nil {
				hasPidsLimit = true
			}
		}
		if !hasPidsLimit {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "missing-pids-limit",
				Message:  fmt.Sprintf("%s: no pids_limit", prefix),
			})
		}

		// Image checks (only for non-build services)
		if svc.Build == nil && svc.Image != "" {
			ref, err := ParseImageRef(svc.Image)
			if err == nil {
				// floating-image-tag
				if ref.IsFloating() {
					findings = append(findings, LintFinding{
						Template: name,
						File:     "docker-compose.yml",
						Severity: SeverityWarning,
						Check:    "floating-image-tag",
						Message:  fmt.Sprintf("%s: image %s uses floating tag %q", prefix, svc.Image, ref.Tag),
					})
				}

				// missing-image-tag (only if parsed as "latest" implicitly)
				if !strings.Contains(svc.Image, ":") {
					findings = append(findings, LintFinding{
						Template: name,
						File:     "docker-compose.yml",
						Severity: SeverityWarning,
						Check:    "missing-image-tag",
						Message:  fmt.Sprintf("%s: image %s has no explicit tag", prefix, svc.Image),
					})
				}
			}
		}

		// build-no-dockerignore
		if svc.Build != nil {
			if !l.templateHasFile(name, ".dockerignore") {
				findings = append(findings, LintFinding{
					Template: name,
					File:     "docker-compose.yml",
					Severity: SeverityWarning,
					Check:    "build-no-dockerignore",
					Message:  fmt.Sprintf("%s: uses build: but no .dockerignore in template", prefix),
				})
			}
		}

		// missing-cap-drop-all
		hasCapDropAll := false
		for _, cap := range svc.CapDrop {
			if strings.EqualFold(cap, "ALL") {
				hasCapDropAll = true
				break
			}
		}
		if !hasCapDropAll {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityInfo,
				Check:    "missing-cap-drop-all",
				Message:  fmt.Sprintf("%s: consider adding cap_drop: [ALL]", prefix),
			})
		}

		// missing-read-only
		if svc.ReadOnly == nil || !*svc.ReadOnly {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityInfo,
				Check:    "missing-read-only",
				Message:  fmt.Sprintf("%s: consider adding read_only: true", prefix),
			})
		}
	}

	// Check networks
	for netName, net := range compose.Networks {
		if !isExternal(net.External) {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml",
				Severity: SeverityWarning,
				Check:    "network-not-external",
				Message:  fmt.Sprintf("network %s is not external", netName),
			})
		}
	}

	return findings
}

// isExternal checks whether the network external field is truthy.
// It can be `true`, `external: true`, or `external: {name: ...}` (legacy).
func isExternal(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case map[string]interface{}:
		return true // legacy format like external: {name: foo}
	}
	return false
}

func (l *Linter) templateHasFile(templateName, fileName string) bool {
	path := templateName + "/" + fileName
	_, err := fs.Stat(l.registry.FS(), path)
	return err == nil
}

// templateRefRe matches Go template references: {{.key}} and {{index . "key"}}
var templateRefRe = regexp.MustCompile(`\{\{\s*(?:\.(\w+)|index\s+\.\s+"(\w+)")\s*\}\}`)

func (l *Linter) lintValueUsage(name string, meta *AppMeta) []LintFinding {
	var findings []LintFinding

	// Collect all defined value names
	definedValues := make(map[string]bool)
	for _, v := range meta.Values {
		definedValues[v.Name] = true
	}

	// Scan all .tmpl files for value references
	usedValues := make(map[string]bool)
	tmplDir := name

	err := fs.WalkDir(l.registry.FS(), tmplDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		data, readErr := fs.ReadFile(l.registry.FS(), path)
		if readErr != nil {
			return nil
		}

		matches := templateRefRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			key := m[1]
			if key == "" {
				key = m[2]
			}
			if key != "" {
				usedValues[key] = true
			}
		}
		return nil
	})
	if err != nil {
		return findings
	}

	// Check for unused values (defined but not referenced)
	for valueName := range definedValues {
		if !usedValues[valueName] {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "app.yaml",
				Severity: SeverityWarning,
				Check:    "unused-value",
				Message:  fmt.Sprintf("Value %q is defined but not referenced in any .tmpl file", valueName),
			})
		}
	}

	// Check for undefined values (referenced but not defined or standard)
	for valueName := range usedValues {
		if !definedValues[valueName] && standardValues[valueName] == "" {
			findings = append(findings, LintFinding{
				Template: name,
				File:     "docker-compose.yml.tmpl",
				Severity: SeverityWarning,
				Check:    "undefined-value",
				Message:  fmt.Sprintf("Value %q is referenced in templates but not defined in app.yaml or standard values", valueName),
			})
		}
	}

	return findings
}

// CountSummary counts findings by severity.
func CountSummary(findings []LintFinding) LintSummary {
	var s LintSummary
	for _, f := range findings {
		switch f.Severity {
		case SeverityError:
			s.Errors++
		case SeverityWarning:
			s.Warnings++
		case SeverityInfo:
			s.Infos++
		}
		s.Total++
	}
	return s
}

func buildResult(findings []LintFinding) *LintResult {
	s := CountSummary(findings)
	return &LintResult{Findings: findings, Summary: s}
}

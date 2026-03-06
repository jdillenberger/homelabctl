package app

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppMeta holds metadata from a template's app.yaml.
type AppMeta struct {
	Name           string          `yaml:"name"`
	Description    string          `yaml:"description"`
	Category       string          `yaml:"category"`
	Version        string          `yaml:"version"`
	Ports          []PortMapping   `yaml:"ports"`
	Volumes        []VolumeMapping `yaml:"volumes"`
	Values         []Value         `yaml:"values"`
	Dependencies   []string        `yaml:"dependencies"`
	HealthCheck    *HealthCheck    `yaml:"health_check"`
	Backup         *BackupMeta     `yaml:"backup"`
	Requirements   *Requirements   `yaml:"requirements"`
	PostDeployInfo *PostDeployInfo `yaml:"post_deploy_info"`
	Hooks          *HooksMeta      `yaml:"hooks"`
	RequiresBuild  bool            `yaml:"requires_build"`
	LintIgnore     []string        `yaml:"lint_ignore"`
}

type PortMapping struct {
	Host        int    `yaml:"host"`
	Container   int    `yaml:"container"`
	Protocol    string `yaml:"protocol"`
	Description string `yaml:"description"`
	ValueName   string `yaml:"value_name"` // name of the value that sets the host port
}

type VolumeMapping struct {
	Name        string `yaml:"name"`
	Container   string `yaml:"container"`
	Description string `yaml:"description"`
}

type Value struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
	Secret      bool   `yaml:"secret"`
	AutoGen     string `yaml:"auto_gen"` // "password", "uuid", etc.
}

type HealthCheck struct {
	URL      string `yaml:"url"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"` // e.g. "10s", defaults to 5s
}

type BackupMeta struct {
	Paths    []string `yaml:"paths"`
	PreHook  string   `yaml:"pre_hook"`
	PostHook string   `yaml:"post_hook"`
}

type Requirements struct {
	MinRAM  string   `yaml:"min_ram"`
	MinDisk string   `yaml:"min_disk"`
	Arch    []string `yaml:"arch"`
}

// PostDeployInfo holds information displayed after a successful deployment.
type PostDeployInfo struct {
	AccessURL   string   `yaml:"access_url"`  // Go template, e.g. "http://{{.hostname}}.{{.domain}}:{{.web_port}}"
	Credentials string   `yaml:"credentials"` // e.g. "admin / value of admin_password"
	Notes       []string `yaml:"notes"`       // First-time setup steps
}

// HooksMeta defines lifecycle hooks for an app.
type HooksMeta struct {
	PostDeploy []Hook `yaml:"post_deploy"`
	PreRemove  []Hook `yaml:"pre_remove"`
}

// Hook defines a single lifecycle hook action.
type Hook struct {
	Type    string `yaml:"type"`    // "http" or "exec"
	URL     string `yaml:"url"`     // For http hooks: URL (Go template)
	Method  string `yaml:"method"`  // For http hooks: HTTP method (default GET)
	Body    string `yaml:"body"`    // For http hooks: request body (Go template)
	Command string `yaml:"command"` // For exec hooks: shell command (Go template)
}

// Registry manages available app templates.
type Registry struct {
	apps   map[string]*AppMeta
	tmplFS fs.FS
}

// NewRegistry scans the given filesystem and loads all app metadata.
func NewRegistry(tmplFS fs.FS) (*Registry, error) {
	r := &Registry{
		apps:   make(map[string]*AppMeta),
		tmplFS: tmplFS,
	}

	entries, err := fs.ReadDir(tmplFS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading templates dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip internal directories (partials, shared templates)
		if strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		appYAML := entry.Name() + "/app.yaml"
		data, err := fs.ReadFile(tmplFS, appYAML)
		if err != nil {
			continue // skip dirs without app.yaml
		}

		var meta AppMeta
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", appYAML, err)
		}

		if meta.Name == "" {
			meta.Name = entry.Name()
		}
		r.apps[meta.Name] = &meta
	}

	return r, nil
}

// Get returns the metadata for a specific app template.
func (r *Registry) Get(name string) (*AppMeta, bool) {
	meta, ok := r.apps[name]
	return meta, ok
}

// List returns all available app template names, sorted.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.apps))
	for name := range r.apps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// All returns all app metadata, sorted by name.
func (r *Registry) All() []*AppMeta {
	names := r.List()
	metas := make([]*AppMeta, len(names))
	for i, name := range names {
		metas[i] = r.apps[name]
	}
	return metas
}

// FS returns the template filesystem.
func (r *Registry) FS() fs.FS {
	return r.tmplFS
}

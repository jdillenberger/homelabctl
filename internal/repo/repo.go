package repo

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jdillenberger/homelabctl/internal/exec"
)

// Repo represents a git repository added as a template source.
type Repo struct {
	Name      string    `yaml:"name"`
	URL       string    `yaml:"url"`
	Ref       string    `yaml:"ref,omitempty"`
	AddedAt   time.Time `yaml:"added_at"`
	UpdatedAt time.Time `yaml:"updated_at,omitempty"`
}

// Manifest holds the list of tracked repos.
type Manifest struct {
	Repos []Repo `yaml:"repos"`
}

// Manager handles repo clone/pull operations and manifest persistence.
type Manager struct {
	reposDir     string
	manifestPath string
	runner       *exec.Runner
}

// NewManager creates a Manager.
func NewManager(reposDir, manifestPath string, runner *exec.Runner) *Manager {
	return &Manager{
		reposDir:     reposDir,
		manifestPath: manifestPath,
		runner:       runner,
	}
}

// Load reads the manifest from disk. Returns an empty manifest if the file
// does not exist.
func (m *Manager) Load() (*Manifest, error) {
	data, err := os.ReadFile(m.manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &manifest, nil
}

// Save writes the manifest to disk.
func (m *Manager) Save(manifest *Manifest) error {
	if err := os.MkdirAll(filepath.Dir(m.manifestPath), 0o755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	return os.WriteFile(m.manifestPath, data, 0o644)
}

// Add clones a git repository and records it in the manifest.
func (m *Manager) Add(url, name, ref string) (*Repo, error) {
	if name == "" {
		name = NameFromURL(url)
	}
	if name == "" {
		return nil, fmt.Errorf("cannot derive repo name from URL %q; use --name", url)
	}

	manifest, err := m.Load()
	if err != nil {
		return nil, err
	}

	for _, r := range manifest.Repos {
		if r.Name == name {
			return nil, fmt.Errorf("repo %q already exists", name)
		}
	}

	dest := filepath.Join(m.reposDir, name)
	if err := os.MkdirAll(m.reposDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating repos directory: %w", err)
	}

	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dest)

	if _, err := m.runner.Run("git", args...); err != nil {
		return nil, fmt.Errorf("git clone: %w", err)
	}

	repo := Repo{
		Name:    name,
		URL:     url,
		Ref:     ref,
		AddedAt: time.Now().UTC().Truncate(time.Second),
	}
	manifest.Repos = append(manifest.Repos, repo)

	if err := m.Save(manifest); err != nil {
		return nil, err
	}
	return &repo, nil
}

// Remove deletes a cloned repo and removes it from the manifest.
func (m *Manager) Remove(name string) error {
	manifest, err := m.Load()
	if err != nil {
		return err
	}

	found := false
	repos := manifest.Repos[:0]
	for _, r := range manifest.Repos {
		if r.Name == name {
			found = true
			continue
		}
		repos = append(repos, r)
	}
	if !found {
		return fmt.Errorf("repo %q not found", name)
	}
	manifest.Repos = repos

	dest := filepath.Join(m.reposDir, name)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("removing repo directory: %w", err)
	}

	return m.Save(manifest)
}

// Update pulls the latest changes for a single repo.
func (m *Manager) Update(name string) error {
	manifest, err := m.Load()
	if err != nil {
		return err
	}

	idx := -1
	for i, r := range manifest.Repos {
		if r.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("repo %q not found", name)
	}

	dest := filepath.Join(m.reposDir, name)
	if _, err := m.runner.Run("git", "-C", dest, "pull"); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}

	manifest.Repos[idx].UpdatedAt = time.Now().UTC().Truncate(time.Second)
	return m.Save(manifest)
}

// UpdateAll pulls the latest changes for all repos.
func (m *Manager) UpdateAll() error {
	manifest, err := m.Load()
	if err != nil {
		return err
	}
	for _, r := range manifest.Repos {
		if err := m.Update(r.Name); err != nil {
			return err
		}
	}
	return nil
}

// List returns all tracked repos.
func (m *Manager) List() ([]Repo, error) {
	manifest, err := m.Load()
	if err != nil {
		return nil, err
	}
	return manifest.Repos, nil
}

// TemplateDirs returns the filesystem paths for all repos in manifest order.
func (m *Manager) TemplateDirs() ([]string, error) {
	manifest, err := m.Load()
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(manifest.Repos))
	for _, r := range manifest.Repos {
		dir := filepath.Join(m.reposDir, r.Name)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// RepoNames returns the repo names in manifest order (for source labelling).
func (m *Manager) RepoNames() ([]string, error) {
	manifest, err := m.Load()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(manifest.Repos))
	for i, r := range manifest.Repos {
		names[i] = r.Name
	}
	return names, nil
}

// NameFromURL derives a repo name from a git URL by stripping .git and taking
// the last path segment.
func NameFromURL(url string) string {
	// Handle SSH-style URLs (git@host:user/repo.git)
	if i := strings.LastIndex(url, ":"); i >= 0 && !strings.Contains(url, "://") {
		url = url[i+1:]
	}

	base := path.Base(url)
	return strings.TrimSuffix(base, ".git")
}

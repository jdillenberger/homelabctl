package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/user/my-templates.git", "my-templates"},
		{"https://github.com/user/my-templates", "my-templates"},
		{"git@github.com:user/my-templates.git", "my-templates"},
		{"git@github.com:user/my-templates", "my-templates"},
		{"https://gitlab.com/org/sub/templates.git", "templates"},
		{"my-repo", "my-repo"},
		{"my-repo.git", "my-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := NameFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("NameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestManifestLoadSave(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")
	mgr := NewManager(dir, manifestPath, nil)

	// Load non-existent manifest should return empty.
	m, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(m.Repos) != 0 {
		t.Fatalf("expected empty repos, got %d", len(m.Repos))
	}

	// Save and reload.
	m.Repos = append(m.Repos, Repo{Name: "test", URL: "https://example.com/test.git"})
	if err := mgr.Save(m); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	m2, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}
	if len(m2.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(m2.Repos))
	}
	if m2.Repos[0].Name != "test" {
		t.Errorf("expected name 'test', got %q", m2.Repos[0].Name)
	}
	if m2.Repos[0].URL != "https://example.com/test.git" {
		t.Errorf("expected URL 'https://example.com/test.git', got %q", m2.Repos[0].URL)
	}
}

func TestManifestLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")
	if err := os.WriteFile(manifestPath, []byte(":::invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(dir, manifestPath, nil)
	_, err := mgr.Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestTemplateDirs(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")

	// Create a fake repo directory.
	repoDir := filepath.Join(dir, "my-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(dir, manifestPath, nil)
	m := &Manifest{
		Repos: []Repo{{Name: "my-repo", URL: "https://example.com/my-repo.git"}},
	}
	if err := mgr.Save(m); err != nil {
		t.Fatal(err)
	}

	dirs, err := mgr.TemplateDirs()
	if err != nil {
		t.Fatalf("TemplateDirs() error: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	if dirs[0] != repoDir {
		t.Errorf("expected %q, got %q", repoDir, dirs[0])
	}
}

func TestTemplateDirsMissingDir(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")

	mgr := NewManager(dir, manifestPath, nil)
	m := &Manifest{
		Repos: []Repo{{Name: "gone", URL: "https://example.com/gone.git"}},
	}
	if err := mgr.Save(m); err != nil {
		t.Fatal(err)
	}

	dirs, err := mgr.TemplateDirs()
	if err != nil {
		t.Fatalf("TemplateDirs() error: %v", err)
	}
	if len(dirs) != 0 {
		t.Fatalf("expected 0 dirs for missing repo, got %d", len(dirs))
	}
}

func TestRepoNames(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")

	mgr := NewManager(dir, manifestPath, nil)
	m := &Manifest{
		Repos: []Repo{
			{Name: "alpha", URL: "https://example.com/alpha.git"},
			{Name: "beta", URL: "https://example.com/beta.git"},
		},
	}
	if err := mgr.Save(m); err != nil {
		t.Fatal(err)
	}

	names, err := mgr.RepoNames()
	if err != nil {
		t.Fatalf("RepoNames() error: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("expected [alpha beta], got %v", names)
	}
}

func TestRemoveNotFound(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "repos.yaml")
	mgr := NewManager(dir, manifestPath, nil)

	if err := mgr.Remove("nonexistent"); err == nil {
		t.Fatal("expected error for removing nonexistent repo")
	}
}

package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// setupMergedDirs creates temporary directories with template subdirectories
// for testing MergedFS.
func setupMergedDirs(t *testing.T) (string, string) {
	t.Helper()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// dir1: repo-alpha with "grafana" and "prometheus" templates
	writeTemplate(t, dir1, "grafana", "name: grafana\ndescription: grafana from alpha\ncategory: monitoring\n")
	writeTemplate(t, dir1, "prometheus", "name: prometheus\ndescription: prometheus from alpha\ncategory: monitoring\n")

	// dir2: repo-beta with "grafana" (conflict) and "loki" templates
	writeTemplate(t, dir2, "grafana", "name: grafana\ndescription: grafana from beta\ncategory: monitoring\n")
	writeTemplate(t, dir2, "loki", "name: loki\ndescription: loki from beta\ncategory: monitoring\n")

	return dir1, dir2
}

func writeTemplate(t *testing.T, base, name, appYAML string) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergedFS_ReadDir(t *testing.T) {
	dir1, dir2 := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1, dir2})

	entries, err := fs.ReadDir(m, ".")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}

	expected := []string{"grafana", "loki", "prometheus"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("entry %d: expected %q, got %q", i, name, names[i])
		}
	}
}

func TestMergedFS_FirstLayerWins(t *testing.T) {
	dir1, dir2 := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1, dir2})

	// grafana exists in both; dir1 (first layer) should win
	data, err := fs.ReadFile(m, "grafana/app.yaml")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if got := string(data); got != "name: grafana\ndescription: grafana from alpha\ncategory: monitoring\n" {
		t.Errorf("expected alpha content, got: %s", got)
	}
}

func TestMergedFS_SecondLayerUnique(t *testing.T) {
	dir1, dir2 := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1, dir2})

	// loki only exists in dir2
	data, err := fs.ReadFile(m, "loki/app.yaml")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if got := string(data); got != "name: loki\ndescription: loki from beta\ncategory: monitoring\n" {
		t.Errorf("expected beta content, got: %s", got)
	}
}

func TestMergedFS_RepoIndex(t *testing.T) {
	dir1, dir2 := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1, dir2})

	tests := []struct {
		name     string
		expected int
	}{
		{"grafana", 0},     // first layer wins
		{"prometheus", 0},  // only in first layer
		{"loki", 1},        // only in second layer
		{"nonexistent", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.RepoIndex(tt.name)
			if got != tt.expected {
				t.Errorf("RepoIndex(%q) = %d, want %d", tt.name, got, tt.expected)
			}
		})
	}
}

func TestMergedFS_NotFound(t *testing.T) {
	dir1, _ := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1})

	_, err := m.Open("nonexistent/app.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}

	_, err = m.ReadFile("nonexistent/app.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}

	_, err = m.ReadDir("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestMergedFS_Empty(t *testing.T) {
	m := NewMergedFS(nil)

	entries, err := fs.ReadDir(m, ".")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestMergedFS_RegistryIntegration(t *testing.T) {
	dir1, dir2 := setupMergedDirs(t)
	m := NewMergedFS([]string{dir1, dir2})

	r, err := NewRegistry(m)
	if err != nil {
		t.Fatalf("NewRegistry error: %v", err)
	}

	names := r.List()
	expected := []string{"grafana", "loki", "prometheus"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d templates, got %d: %v", len(expected), len(names), names)
	}

	// grafana should come from first layer
	meta, ok := r.Get("grafana")
	if !ok {
		t.Fatal("expected grafana in registry")
	}
	if meta.Description != "grafana from alpha" {
		t.Errorf("expected alpha description, got %q", meta.Description)
	}
}

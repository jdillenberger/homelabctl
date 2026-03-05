package app

import (
	"io/fs"
	"sort"
	"testing"
	"testing/fstest"
)

func newTestOverlay(t *testing.T) *OverlayFS {
	t.Helper()

	lower := fstest.MapFS{
		"adguard/app.yaml":                 {Data: []byte("name: adguard\ndescription: built-in adguard\ncategory: infra\n")},
		"adguard/docker-compose.yml.tmpl":  {Data: []byte("image: adguard-lower")},
		"nextcloud/app.yaml":               {Data: []byte("name: nextcloud\ndescription: built-in nextcloud\ncategory: productivity\n")},
		"nextcloud/docker-compose.yml.tmpl": {Data: []byte("image: nextcloud-lower")},
	}

	upper := fstest.MapFS{
		"adguard/app.yaml":                {Data: []byte("name: adguard\ndescription: custom adguard\ncategory: infra\n")},
		"adguard/docker-compose.yml.tmpl": {Data: []byte("image: adguard-upper")},
		"myapp/app.yaml":                  {Data: []byte("name: myapp\ndescription: my custom app\ncategory: custom\n")},
		"myapp/docker-compose.yml.tmpl":   {Data: []byte("image: myapp")},
	}

	return NewOverlayFS(lower, upper)
}

func TestOverlayFS_ReadDir_Root(t *testing.T) {
	o := newTestOverlay(t)

	entries, err := fs.ReadDir(o, ".")
	if err != nil {
		t.Fatalf("ReadDir(.) error: %v", err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}

	expected := []string{"adguard", "myapp", "nextcloud"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("entry %d: expected %q, got %q", i, name, names[i])
		}
	}

	if !sort.StringsAreSorted(names) {
		t.Errorf("entries should be sorted, got: %v", names)
	}
}

func TestOverlayFS_OverrideTemplate(t *testing.T) {
	o := newTestOverlay(t)

	// adguard exists in both; upper should win
	data, err := fs.ReadFile(o, "adguard/docker-compose.yml.tmpl")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "image: adguard-upper" {
		t.Errorf("expected upper content, got: %s", string(data))
	}
}

func TestOverlayFS_FallbackToLower(t *testing.T) {
	o := newTestOverlay(t)

	// nextcloud only exists in lower
	data, err := fs.ReadFile(o, "nextcloud/docker-compose.yml.tmpl")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "image: nextcloud-lower" {
		t.Errorf("expected lower content, got: %s", string(data))
	}
}

func TestOverlayFS_LocalOnlyTemplate(t *testing.T) {
	o := newTestOverlay(t)

	// myapp only exists in upper
	data, err := fs.ReadFile(o, "myapp/docker-compose.yml.tmpl")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "image: myapp" {
		t.Errorf("expected upper content, got: %s", string(data))
	}
}

func TestOverlayFS_Source(t *testing.T) {
	o := newTestOverlay(t)

	tests := []struct {
		name     string
		expected string
	}{
		{"nextcloud", "built-in"},
		{"myapp", "local"},
		{"adguard", "override"},
		{"nonexistent", "built-in"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := o.Source(tt.name)
			if source != tt.expected {
				t.Errorf("Source(%q) = %q, want %q", tt.name, source, tt.expected)
			}
		})
	}
}

func TestOverlayFS_RegistryIntegration(t *testing.T) {
	o := newTestOverlay(t)

	r, err := NewRegistry(o)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	// Should see all templates from both layers
	names := r.List()
	expected := []string{"adguard", "myapp", "nextcloud"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d templates, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("template %d: expected %q, got %q", i, name, names[i])
		}
	}

	// adguard should have the overridden description
	meta, ok := r.Get("adguard")
	if !ok {
		t.Fatal("expected adguard to be loaded")
	}
	if meta.Description != "custom adguard" {
		t.Errorf("expected overridden description, got: %q", meta.Description)
	}

	// nextcloud should have the built-in description
	meta, ok = r.Get("nextcloud")
	if !ok {
		t.Fatal("expected nextcloud to be loaded")
	}
	if meta.Description != "built-in nextcloud" {
		t.Errorf("expected built-in description, got: %q", meta.Description)
	}
}

func TestOverlayFS_NilUpper(t *testing.T) {
	lower := fstest.MapFS{
		"adguard/app.yaml": {Data: []byte("name: adguard\ncategory: infra\n")},
	}

	o := NewOverlayFS(lower, nil)

	entries, err := fs.ReadDir(o, ".")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name() != "adguard" {
		t.Errorf("expected adguard, got %s", entries[0].Name())
	}

	if o.Source("adguard") != "built-in" {
		t.Errorf("expected built-in source with nil upper")
	}
}

func TestOverlayFS_WalkDir(t *testing.T) {
	o := newTestOverlay(t)

	// Walk the overridden adguard template — should see upper files
	var paths []string
	err := fs.WalkDir(o, "adguard", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir error: %v", err)
	}

	// Should contain the dir and the two files from upper
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d: %v", len(paths), paths)
	}
}

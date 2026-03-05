package app

import (
	"sort"
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestNewRegistry(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	names := r.List()
	if len(names) == 0 {
		t.Fatal("NewRegistry() loaded 0 templates, expected at least 1")
	}

	// We know the embedded FS contains adguard, portainer, nextcloud
	expected := []string{"adguard", "nextcloud", "portainer"}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected template %q to be loaded", name)
		}
	}
}

func TestRegistryGet(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	t.Run("existing app returns correct metadata", func(t *testing.T) {
		meta, ok := r.Get("adguard")
		if !ok {
			t.Fatal("Get(adguard) returned false")
		}
		if meta.Name != "adguard" {
			t.Errorf("expected Name=adguard, got %q", meta.Name)
		}
		if meta.Description == "" {
			t.Error("expected non-empty Description")
		}
		if meta.Category != "infra" {
			t.Errorf("expected Category=infra, got %q", meta.Category)
		}
	})

	t.Run("unknown app returns false", func(t *testing.T) {
		_, ok := r.Get("nonexistent-app")
		if ok {
			t.Error("Get(nonexistent-app) returned true, expected false")
		}
	})
}

func TestRegistryList(t *testing.T) {
	r, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	names := r.List()

	t.Run("returns non-empty list", func(t *testing.T) {
		if len(names) == 0 {
			t.Fatal("List() returned empty slice")
		}
	})

	t.Run("list is sorted", func(t *testing.T) {
		if !sort.StringsAreSorted(names) {
			t.Errorf("List() returned unsorted names: %v", names)
		}
	})

	t.Run("contains known apps", func(t *testing.T) {
		nameSet := make(map[string]bool)
		for _, n := range names {
			nameSet[n] = true
		}
		for _, expected := range []string{"adguard", "portainer", "nextcloud"} {
			if !nameSet[expected] {
				t.Errorf("List() missing expected app %q", expected)
			}
		}
	})
}

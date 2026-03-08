package doctor

import (
	"testing"
)

func TestCheckAll(t *testing.T) {
	results := CheckAll()
	deps := DefaultDependencies()

	// CheckAll returns dependency checks + traefik-mdns + peer-scanner optional checks
	expectedCount := len(deps) + 2

	t.Run("returns result for each check", func(t *testing.T) {
		if len(results) != expectedCount {
			t.Errorf("CheckAll() returned %d results, expected %d", len(results), expectedCount)
		}
	})

	t.Run("each result has a name", func(t *testing.T) {
		for i, r := range results {
			if r.Name == "" {
				t.Errorf("result[%d] has empty Name", i)
			}
		}
	})

	t.Run("dependency results match order", func(t *testing.T) {
		for i, dep := range deps {
			if i >= len(results) {
				break
			}
			if results[i].Name != dep.Name {
				t.Errorf("result[%d].Name = %q, expected %q", i, results[i].Name, dep.Name)
			}
		}
	})

	t.Run("traefik-mdns check is appended", func(t *testing.T) {
		if len(results) < len(deps)+1 {
			t.Fatal("not enough results for traefik-mdns check")
		}
		idx := len(deps)
		if results[idx].Name != "traefik-mdns" {
			t.Errorf("expected traefik-mdns at index %d, got %q", idx, results[idx].Name)
		}
	})

	t.Run("peer-scanner check is appended", func(t *testing.T) {
		if len(results) < len(deps)+2 {
			t.Fatal("not enough results for peer-scanner check")
		}
		idx := len(deps) + 1
		if results[idx].Name != "peer-scanner" {
			t.Errorf("expected peer-scanner at index %d, got %q", idx, results[idx].Name)
		}
	})
}

func TestCheck(t *testing.T) {
	t.Run("nonexistent binary is not installed", func(t *testing.T) {
		dep := Dependency{
			Name:           "fake-tool",
			Binary:         "this-binary-definitely-does-not-exist-xyz",
			VersionArgs:    []string{"--version"},
			InstallCommand: "apt install -y fake-tool",
		}
		result := Check(dep)
		if result.Installed {
			t.Error("expected Installed=false for nonexistent binary")
		}
		if result.Name != "fake-tool" {
			t.Errorf("expected Name=fake-tool, got %q", result.Name)
		}
		if result.InstallCommand != "apt install -y fake-tool" {
			t.Errorf("expected InstallCommand preserved, got %q", result.InstallCommand)
		}
	})

	t.Run("existing binary is detected", func(t *testing.T) {
		// /bin/sh should exist on any linux system
		dep := Dependency{
			Name:           "sh",
			Binary:         "sh",
			VersionArgs:    nil,
			InstallCommand: "",
		}
		result := Check(dep)
		if !result.Installed {
			t.Error("expected sh to be installed")
		}
	})
}

func TestDefaultDependencies(t *testing.T) {
	deps := DefaultDependencies()

	if len(deps) == 0 {
		t.Fatal("DefaultDependencies() returned empty list")
	}

	names := make(map[string]bool)
	for _, d := range deps {
		if d.Name == "" {
			t.Error("dependency has empty Name")
		}
		if d.Binary == "" {
			t.Errorf("dependency %q has empty Binary", d.Name)
		}
		if d.InstallCommand == "" {
			t.Errorf("dependency %q has empty InstallCommand", d.Name)
		}
		names[d.Name] = true
	}

	// Verify key dependencies are present
	for _, expected := range []string{"docker", "borgmatic", "borg"} {
		if !names[expected] {
			t.Errorf("expected dependency %q not found", expected)
		}
	}
}

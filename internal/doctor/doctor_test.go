package doctor

import (
	"testing"
)

func TestCheckAll(t *testing.T) {
	results := CheckAll()
	deps := DefaultDependencies()

	t.Run("returns result for each dependency", func(t *testing.T) {
		if len(results) != len(deps) {
			t.Errorf("CheckAll() returned %d results, expected %d", len(results), len(deps))
		}
	})

	t.Run("each result has name and install command", func(t *testing.T) {
		for i, r := range results {
			if r.Name == "" {
				t.Errorf("result[%d] has empty Name", i)
			}
			if r.InstallCommand == "" {
				t.Errorf("result[%d] (%s) has empty InstallCommand", i, r.Name)
			}
		}
	})

	t.Run("results match dependency order", func(t *testing.T) {
		for i, r := range results {
			if r.Name != deps[i].Name {
				t.Errorf("result[%d].Name = %q, expected %q", i, r.Name, deps[i].Name)
			}
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

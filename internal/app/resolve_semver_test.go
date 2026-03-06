package app

import (
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input   string
		major   int
		minor   int
		patch   int
		pre     string
		wantErr bool
	}{
		{"1.2.3", 1, 2, 3, "", false},
		{"v1.2.3", 1, 2, 3, "", false},
		{"32.0.6", 32, 0, 6, "", false},
		{"v0.1.0", 0, 1, 0, "", false},
		{"1.2.3-beta", 1, 2, 3, "beta", false},
		{"v1.2.3-rc.1", 1, 2, 3, "rc.1", false},
		{"1.0.0-alpha.1", 1, 0, 0, "alpha.1", false},
		// Invalid inputs
		{"latest", 0, 0, 0, "", true},
		{"release", 0, 0, 0, "", true},
		{"16-alpine", 0, 0, 0, "", true},
		{"abc", 0, 0, 0, "", true},
		{"1.2", 0, 0, 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := ParseSemver(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSemver(%q) expected error, got %v", tt.input, v)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSemver(%q) unexpected error: %v", tt.input, err)
			}
			if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch {
				t.Errorf("ParseSemver(%q) = %d.%d.%d, want %d.%d.%d",
					tt.input, v.Major, v.Minor, v.Patch, tt.major, tt.minor, tt.patch)
			}
			if v.Pre != tt.pre {
				t.Errorf("ParseSemver(%q).Pre = %q, want %q", tt.input, v.Pre, tt.pre)
			}
		})
	}
}

func TestSemVerString(t *testing.T) {
	tests := []struct {
		ver    SemVer
		expect string
	}{
		{SemVer{1, 2, 3, ""}, "1.2.3"},
		{SemVer{32, 0, 6, ""}, "32.0.6"},
		{SemVer{1, 0, 0, "beta"}, "1.0.0-beta"},
		{SemVer{0, 1, 0, "rc.1"}, "0.1.0-rc.1"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if got := tt.ver.String(); got != tt.expect {
				t.Errorf("SemVer.String() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name string
		a, b SemVer
		want int
	}{
		{"equal", SemVer{1, 2, 3, ""}, SemVer{1, 2, 3, ""}, 0},
		{"major less", SemVer{1, 0, 0, ""}, SemVer{2, 0, 0, ""}, -1},
		{"major greater", SemVer{2, 0, 0, ""}, SemVer{1, 0, 0, ""}, 1},
		{"minor less", SemVer{1, 2, 0, ""}, SemVer{1, 3, 0, ""}, -1},
		{"minor greater", SemVer{1, 3, 0, ""}, SemVer{1, 2, 0, ""}, 1},
		{"patch less", SemVer{1, 2, 3, ""}, SemVer{1, 2, 4, ""}, -1},
		{"patch greater", SemVer{1, 2, 4, ""}, SemVer{1, 2, 3, ""}, 1},
		{"pre < release", SemVer{1, 0, 0, "beta"}, SemVer{1, 0, 0, ""}, -1},
		{"release > pre", SemVer{1, 0, 0, ""}, SemVer{1, 0, 0, "beta"}, 1},
		{"pre equal", SemVer{1, 0, 0, "alpha"}, SemVer{1, 0, 0, "alpha"}, 0},
		{"pre lexicographic", SemVer{1, 0, 0, "alpha"}, SemVer{1, 0, 0, "beta"}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareSemver(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareSemver(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestUpgradeType(t *testing.T) {
	tests := []struct {
		name     string
		from, to SemVer
		want     string
	}{
		{"patch", SemVer{1, 2, 3, ""}, SemVer{1, 2, 4, ""}, "patch"},
		{"minor", SemVer{1, 2, 3, ""}, SemVer{1, 3, 0, ""}, "minor"},
		{"major", SemVer{1, 2, 3, ""}, SemVer{2, 0, 0, ""}, "major"},
		{"patch same minor", SemVer{32, 0, 6, ""}, SemVer{32, 0, 9, ""}, "patch"},
		{"minor different", SemVer{32, 0, 6, ""}, SemVer{32, 1, 0, ""}, "minor"},
		{"major different", SemVer{32, 0, 6, ""}, SemVer{33, 0, 0, ""}, "major"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpgradeType(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("UpgradeType(%v, %v) = %q, want %q", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestScanAllImages(t *testing.T) {
	entries, err := ScanAllImages(templates.FS)
	if err != nil {
		t.Fatalf("ScanAllImages() error: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("ScanAllImages() returned no entries")
	}

	// Should include both floating and pinned tags
	hasFloating := false
	hasPinned := false
	for _, e := range entries {
		if e.Ref.IsFloating() {
			hasFloating = true
		} else {
			hasPinned = true
		}
	}

	if !hasFloating {
		t.Error("ScanAllImages() returned no floating tag entries")
	}
	if !hasPinned {
		t.Error("ScanAllImages() returned no pinned tag entries")
	}

	// Should return more entries than ScanFloatingTags
	floating, err := ScanFloatingTags(templates.FS)
	if err != nil {
		t.Fatalf("ScanFloatingTags() error: %v", err)
	}
	if len(entries) <= len(floating) {
		t.Errorf("ScanAllImages() returned %d entries, expected more than ScanFloatingTags() which returned %d",
			len(entries), len(floating))
	}

	// Verify no template-expanded images are included
	for _, e := range entries {
		if e.AppName == "webtop" {
			t.Errorf("webtop should be excluded (dynamic tag), but found: %s", e.Image)
		}
	}
}

func TestScanDeployedImages(t *testing.T) {
	compose := []byte(`
services:
  app:
    image: nextcloud:32.0.6
  db:
    image: postgres:16-alpine
  redis:
    image: redis:7.4.2
`)
	refs, err := ScanDeployedImages(compose)
	if err != nil {
		t.Fatalf("ScanDeployedImages() error: %v", err)
	}

	if len(refs) != 3 {
		t.Fatalf("ScanDeployedImages() returned %d refs, want 3", len(refs))
	}

	expected := []struct {
		repo string
		tag  string
	}{
		{"nextcloud", "32.0.6"},
		{"postgres", "16-alpine"},
		{"redis", "7.4.2"},
	}

	for i, e := range expected {
		if refs[i].Repo != e.repo {
			t.Errorf("ref[%d].Repo = %q, want %q", i, refs[i].Repo, e.repo)
		}
		if refs[i].Tag != e.tag {
			t.Errorf("ref[%d].Tag = %q, want %q", i, refs[i].Tag, e.tag)
		}
	}
}

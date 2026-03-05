package app

import (
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		input     string
		registry  string
		namespace string
		repo      string
		tag       string
	}{
		{
			input:     "nginx:1.25",
			registry:  "docker.io",
			namespace: "library",
			repo:      "nginx",
			tag:       "1.25",
		},
		{
			input:     "gitea/gitea:1.25.4",
			registry:  "docker.io",
			namespace: "gitea",
			repo:      "gitea",
			tag:       "1.25.4",
		},
		{
			input:     "ghcr.io/immich-app/immich-server:release",
			registry:  "ghcr.io",
			namespace: "immich-app",
			repo:      "immich-server",
			tag:       "release",
		},
		{
			input:     "lscr.io/linuxserver/obsidian:latest",
			registry:  "lscr.io",
			namespace: "linuxserver",
			repo:      "obsidian",
			tag:       "latest",
		},
		{
			input:     "postgres:16-alpine",
			registry:  "docker.io",
			namespace: "library",
			repo:      "postgres",
			tag:       "16-alpine",
		},
		{
			input:     "redis",
			registry:  "docker.io",
			namespace: "library",
			repo:      "redis",
			tag:       "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref, err := ParseImageRef(tt.input)
			if err != nil {
				t.Fatalf("ParseImageRef(%q) error: %v", tt.input, err)
			}
			if ref.Registry != tt.registry {
				t.Errorf("registry: got %q, want %q", ref.Registry, tt.registry)
			}
			if ref.Namespace != tt.namespace {
				t.Errorf("namespace: got %q, want %q", ref.Namespace, tt.namespace)
			}
			if ref.Repo != tt.repo {
				t.Errorf("repo: got %q, want %q", ref.Repo, tt.repo)
			}
			if ref.Tag != tt.tag {
				t.Errorf("tag: got %q, want %q", ref.Tag, tt.tag)
			}
		})
	}
}

func TestImageRefIsFloating(t *testing.T) {
	tests := []struct {
		tag      string
		floating bool
	}{
		{"latest", true},
		{"release", true},
		{"stable", true},
		{"v1.2.3", false},
		{"1.25.4", false},
		{"16-alpine", false},
	}

	for _, tt := range tests {
		ref := ImageRef{Tag: tt.tag}
		if got := ref.IsFloating(); got != tt.floating {
			t.Errorf("ImageRef{Tag: %q}.IsFloating() = %v, want %v", tt.tag, got, tt.floating)
		}
	}
}

func TestImageRefString(t *testing.T) {
	tests := []struct {
		ref    ImageRef
		expect string
	}{
		{
			ref:    ImageRef{Registry: "docker.io", Namespace: "library", Repo: "nginx", Tag: "1.25"},
			expect: "nginx:1.25",
		},
		{
			ref:    ImageRef{Registry: "docker.io", Namespace: "gitea", Repo: "gitea", Tag: "1.25.4"},
			expect: "gitea/gitea:1.25.4",
		},
		{
			ref:    ImageRef{Registry: "ghcr.io", Namespace: "immich-app", Repo: "immich-server", Tag: "release"},
			expect: "ghcr.io/immich-app/immich-server:release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if got := tt.ref.String(); got != tt.expect {
				t.Errorf("String() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestScanFloatingTags(t *testing.T) {
	entries, err := ScanFloatingTags(templates.FS)
	if err != nil {
		t.Fatalf("ScanFloatingTags() error: %v", err)
	}

	// We know there are floating tags in immich (release), openclaw (latest), obsidian (latest)
	if len(entries) == 0 {
		t.Fatal("ScanFloatingTags() returned no entries, expected at least 3 apps with floating tags")
	}

	// Check that known floating tags are found
	foundApps := make(map[string]bool)
	for _, e := range entries {
		foundApps[e.AppName] = true
	}

	for _, expected := range []string{"immich", "openclaw", "obsidian"} {
		if !foundApps[expected] {
			t.Errorf("expected floating tag entry for %q", expected)
		}
	}

	// Verify no template-expanded images (like webtop's {{index . "desktop_env"}}) are included
	for _, e := range entries {
		if e.AppName == "webtop" {
			t.Errorf("webtop should be excluded (dynamic tag), but found: %s", e.Image)
		}
	}
}

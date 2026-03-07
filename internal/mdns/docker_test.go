package mdns

import (
	"testing"
)

func TestExtractHosts(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want []string
	}{
		{
			name: "single host",
			rule: "Host(`app.example.local`)",
			want: []string{"app.example.local"},
		},
		{
			name: "multiple hosts with OR",
			rule: "Host(`a.local`) || Host(`b.local`)",
			want: []string{"a.local", "b.local"},
		},
		{
			name: "host with path",
			rule: "Host(`app.local`) && PathPrefix(`/api`)",
			want: []string{"app.local"},
		},
		{
			name: "no host",
			rule: "PathPrefix(`/api`)",
			want: nil,
		},
		{
			name: "empty rule",
			rule: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHosts(tt.rule)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractHosts(%q) = %v, want %v", tt.rule, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractHosts(%q)[%d] = %q, want %q", tt.rule, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsTraefikRouterRule(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"traefik.http.routers.myapp.rule", true},
		{"traefik.http.routers.my-app-secure.rule", true},
		{"traefik.http.routers..rule", false},
		{"traefik.http.services.myapp.loadbalancer.server.port", false},
		{"traefik.enable", false},
		{"traefik.http.routers.myapp.entrypoints", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isTraefikRouterRule(tt.key); got != tt.want {
				t.Errorf("isTraefikRouterRule(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

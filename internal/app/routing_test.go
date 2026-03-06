package app

import (
	"strings"
	"testing"
)

func TestBuildLabelsHTTPOnly(t *testing.T) {
	l := &RoutingLabeler{
		domain:       "myhost.local",
		httpsEnabled: false,
	}
	routing := &DeployedRouting{
		Domains:       []string{"app.myhost.local"},
		ContainerPort: 8080,
	}

	labels := l.buildLabels("myapp", routing)

	assertLabel(t, labels, "traefik.enable", "true")
	assertLabel(t, labels, "traefik.http.routers.myapp.entrypoints", "web")
	assertLabel(t, labels, "traefik.http.routers.myapp.rule", "Host(`app.myhost.local`)")
	assertLabel(t, labels, "traefik.http.services.myapp.loadbalancer.server.port", "8080")

	// Should NOT have any secure router
	for k := range labels {
		if strings.Contains(k, "secure") {
			t.Errorf("unexpected secure label: %s", k)
		}
	}
}

func TestBuildLabelsHTTPSLocalOnly(t *testing.T) {
	l := &RoutingLabeler{
		domain:       "myhost.local",
		httpsEnabled: true,
		acmeEmail:    "",
	}
	routing := &DeployedRouting{
		Domains:       []string{"app.myhost.local"},
		ContainerPort: 8080,
	}

	labels := l.buildLabels("myapp", routing)

	// HTTP router with redirect
	assertLabel(t, labels, "traefik.http.routers.myapp.middlewares", "myapp-redirect")
	assertLabel(t, labels, "traefik.http.middlewares.myapp-redirect.redirectscheme.scheme", "https")

	// Single secure router (all local) — no certresolver
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.entrypoints", "websecure")
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.tls", "true")
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.service", "myapp")

	// Should NOT have certresolver
	for k := range labels {
		if strings.Contains(k, "certresolver") {
			t.Errorf("local-only should not have certresolver label: %s=%s", k, labels[k])
		}
	}
}

func TestBuildLabelsHTTPSExternalWithACME(t *testing.T) {
	l := &RoutingLabeler{
		domain:       "example.com",
		httpsEnabled: true,
		acmeEmail:    "user@example.com",
	}
	routing := &DeployedRouting{
		Domains:       []string{"app.example.com"},
		ContainerPort: 3000,
	}

	labels := l.buildLabels("myapp", routing)

	// Single secure router (all external) — with certresolver
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.tls", "true")
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.tls.certresolver", "letsencrypt")
}

func TestBuildLabelsHTTPSExternalWithoutACME(t *testing.T) {
	l := &RoutingLabeler{
		domain:       "example.com",
		httpsEnabled: true,
		acmeEmail:    "",
	}
	routing := &DeployedRouting{
		Domains:       []string{"app.example.com"},
		ContainerPort: 3000,
	}

	labels := l.buildLabels("myapp", routing)

	// Secure router without certresolver (fallback to file provider)
	assertLabel(t, labels, "traefik.http.routers.myapp-secure.tls", "true")
	for k := range labels {
		if strings.Contains(k, "certresolver") {
			t.Errorf("no-ACME should not have certresolver: %s", k)
		}
	}
}

func TestBuildLabelsMixedDomains(t *testing.T) {
	l := &RoutingLabeler{
		domain:       "myhost.local",
		httpsEnabled: true,
		acmeEmail:    "user@example.com",
	}
	routing := &DeployedRouting{
		Domains:       []string{"app.myhost.local", "app.example.com"},
		ContainerPort: 8080,
	}

	labels := l.buildLabels("myapp", routing)

	// Should have separate routers for local and external
	assertLabel(t, labels, "traefik.http.routers.myapp-local-secure.tls", "true")
	assertLabel(t, labels, "traefik.http.routers.myapp-local-secure.rule", "Host(`app.myhost.local`)")

	assertLabel(t, labels, "traefik.http.routers.myapp-ext-secure.tls", "true")
	assertLabel(t, labels, "traefik.http.routers.myapp-ext-secure.tls.certresolver", "letsencrypt")
	assertLabel(t, labels, "traefik.http.routers.myapp-ext-secure.rule", "Host(`app.example.com`)")

	// Local router should NOT have certresolver
	for k := range labels {
		if strings.Contains(k, "local-secure") && strings.Contains(k, "certresolver") {
			t.Errorf("local router should not have certresolver: %s", k)
		}
	}
}

func TestIsLocalDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"app.myhost.local", true},
		{"myhost.local", true},
		{"app.example.com", false},
		{"app.local.example.com", false},
	}
	for _, tt := range tests {
		if got := isLocalDomain(tt.domain); got != tt.want {
			t.Errorf("isLocalDomain(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func assertLabel(t *testing.T, labels map[string]string, key, want string) {
	t.Helper()
	got, ok := labels[key]
	if !ok {
		t.Errorf("missing label %q", key)
		return
	}
	if got != want {
		t.Errorf("label %q = %q, want %q", key, got, want)
	}
}

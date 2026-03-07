package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdillenberger/homelabctl/internal/config"
	"gopkg.in/yaml.v3"
)

// RoutingLabeler injects Traefik labels into docker-compose YAML.
type RoutingLabeler struct {
	domain       string
	httpsEnabled bool
	acmeEmail    string
	network      string
}

// NewRoutingLabeler creates a labeler from the current config.
func NewRoutingLabeler(cfg *config.Config) *RoutingLabeler {
	return &RoutingLabeler{
		domain:       cfg.RoutingDomain(),
		httpsEnabled: cfg.Routing.HTTPS.Enabled,
		acmeEmail:    cfg.Routing.HTTPS.AcmeEmail,
		network:      cfg.Docker.DefaultNetwork,
	}
}

// InjectLabels parses docker-compose YAML, adds Traefik labels to the primary
// service, optionally removes host port bindings, and returns modified YAML.
func (l *RoutingLabeler) InjectLabels(composeYAML, appName string, routing *DeployedRouting) (string, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeYAML), &doc); err != nil {
		return "", fmt.Errorf("parsing compose YAML: %w", err)
	}

	servicesRaw, ok := doc["services"]
	if !ok {
		return composeYAML, nil
	}
	services, ok := servicesRaw.(map[string]interface{})
	if !ok {
		return composeYAML, nil
	}

	// Find primary service: first one with ports, or first one matching appName
	var primaryName string
	var primarySvc map[string]interface{}

	for name, svcRaw := range services {
		svc, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if primarySvc == nil {
			primaryName = name
			primarySvc = svc
		}
		// Prefer service with container_name matching appName
		if cn, ok := svc["container_name"].(string); ok && cn == appName {
			primaryName = name
			primarySvc = svc
			break
		}
		// Or first service with ports
		if _, hasPorts := svc["ports"]; hasPorts && primarySvc != nil && primaryName != name {
			if _, alreadyHasPorts := primarySvc["ports"]; !alreadyHasPorts {
				primaryName = name
				primarySvc = svc
			}
		}
	}

	if primarySvc == nil {
		return composeYAML, nil
	}

	// Build Traefik labels
	labels := l.buildLabels(appName, routing)

	// Merge labels into service
	existingLabels := getLabelsMap(primarySvc)
	for k, v := range labels {
		existingLabels[k] = v
	}

	// Convert to list format for docker-compose
	var labelsList []string
	for k, v := range existingLabels {
		labelsList = append(labelsList, k+"="+v)
	}
	primarySvc["labels"] = labelsList

	// Remove ports if KeepPorts is false
	if !routing.KeepPorts {
		delete(primarySvc, "ports")
	}

	services[primaryName] = primarySvc
	doc["services"] = services

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshalling compose YAML: %w", err)
	}

	return string(out), nil
}

func (l *RoutingLabeler) buildLabels(appName string, routing *DeployedRouting) map[string]string {
	// Sanitize app name for use in Traefik router names
	routerName := strings.ReplaceAll(appName, ".", "-")
	routerName = strings.ReplaceAll(routerName, "_", "-")

	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName): fmt.Sprintf("%d", routing.ContainerPort),
	}

	// Build Host rule for all domains
	var allHostParts []string
	for _, d := range routing.Domains {
		allHostParts = append(allHostParts, fmt.Sprintf("Host(`%s`)", d))
	}
	allRule := strings.Join(allHostParts, " || ")

	// HTTP router (all domains)
	labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "web"
	labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = allRule

	if !l.httpsEnabled {
		return labels
	}

	// HTTP -> HTTPS redirect
	labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", routerName)] = routerName + "-redirect"
	labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.scheme", routerName)] = "https"
	labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.permanent", routerName)] = "true"

	// Classify domains
	var localDomains, externalDomains []string
	for _, d := range routing.Domains {
		if isLocalDomain(d) {
			localDomains = append(localDomains, d)
		} else {
			externalDomains = append(externalDomains, d)
		}
	}

	// If all domains are the same type, use a single secure router
	switch {
	case len(externalDomains) == 0:
		// All local — use file provider cert (tls=true, no certresolver)
		l.addSecureRouter(labels, routerName, routerName+"-secure", allRule, false)
	case len(localDomains) == 0:
		// All external
		l.addSecureRouter(labels, routerName, routerName+"-secure", allRule, l.acmeEmail != "")
	default:
		// Mixed: separate routers for local and external domains
		var localParts []string
		for _, d := range localDomains {
			localParts = append(localParts, fmt.Sprintf("Host(`%s`)", d))
		}
		localRule := strings.Join(localParts, " || ")
		l.addSecureRouter(labels, routerName, routerName+"-local-secure", localRule, false)

		var extParts []string
		for _, d := range externalDomains {
			extParts = append(extParts, fmt.Sprintf("Host(`%s`)", d))
		}
		extRule := strings.Join(extParts, " || ")
		l.addSecureRouter(labels, routerName, routerName+"-ext-secure", extRule, l.acmeEmail != "")
	}

	return labels
}

// addSecureRouter adds labels for a HTTPS router. If useACME is true, it uses
// the letsencrypt certresolver; otherwise it relies on the file provider cert.
func (l *RoutingLabeler) addSecureRouter(labels map[string]string, serviceName, routerName, rule string, useACME bool) {
	labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "websecure"
	labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = rule
	labels[fmt.Sprintf("traefik.http.routers.%s.tls", routerName)] = "true"
	labels[fmt.Sprintf("traefik.http.routers.%s.service", routerName)] = serviceName
	if useACME {
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = "letsencrypt"
	}
}

// isLocalDomain returns true if the domain ends with ".local".
func isLocalDomain(domain string) bool {
	return strings.HasSuffix(domain, ".local")
}

func getLabelsMap(svc map[string]interface{}) map[string]string {
	result := make(map[string]string)
	raw, ok := svc["labels"]
	if !ok {
		return result
	}

	switch v := raw.(type) {
	case map[string]interface{}:
		for k, val := range v {
			result[k] = fmt.Sprintf("%v", val)
		}
	case []interface{}:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			parts := strings.SplitN(s, "=", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			}
		}
	}
	return result
}

// computeRouting builds a DeployedRouting from config and app metadata.
// mergedValues is checked for a user-supplied "routing_hostname" override.
func computeRouting(cfg *config.Config, appName string, meta *AppMeta, mergedValues map[string]string) *DeployedRouting {
	r := &DeployedRouting{
		Enabled:   true,
		KeepPorts: true,
	}

	// Check if app explicitly disables routing
	if meta.Routing != nil && meta.Routing.Enabled != nil && !*meta.Routing.Enabled {
		r.Enabled = false
		return r
	}

	// Determine subdomain
	subdomain := appName
	if meta.Routing != nil && meta.Routing.Subdomain != "" {
		subdomain = meta.Routing.Subdomain
	}

	// Determine domain with priority:
	// 1. User override via --set routing_hostname=X → X.network_domain
	// 2. App meta routing.hostname → hostname.network_domain
	// 3. Explicit routing.domain in config → subdomain.routing_domain (hierarchical)
	// 4. Default → subdomain-hostname.network_domain (flat)
	switch {
	case mergedValues["routing_hostname"] != "":
		r.Domains = []string{mergedValues["routing_hostname"] + "." + cfg.Network.Domain}
	case meta.Routing != nil && meta.Routing.Hostname != "":
		r.Domains = []string{meta.Routing.Hostname + "." + cfg.Network.Domain}
	case cfg.Routing.Domain != "":
		// User has explicit routing.domain set — they have DNS, use hierarchical naming
		r.Domains = []string{subdomain + "." + cfg.Routing.Domain}
	default:
		// Default: flat naming for mDNS compatibility
		r.Domains = []string{subdomain + "-" + cfg.Hostname + "." + cfg.Network.Domain}
	}

	// Determine container port
	switch {
	case meta.Routing != nil && meta.Routing.ContainerPort > 0:
		r.ContainerPort = meta.Routing.ContainerPort
	case len(meta.Ports) > 0:
		r.ContainerPort = meta.Ports[0].Container
	default:
		r.ContainerPort = 80
	}

	// Determine KeepPorts
	if meta.Routing != nil && meta.Routing.KeepPorts != nil {
		r.KeepPorts = *meta.Routing.KeepPorts
	}

	return r
}

// RegenerateCompose re-renders the template for a deployed app and re-injects
// routing labels if applicable. It writes the updated compose file and runs
// docker compose up -d.
func (m *Manager) RegenerateCompose(appName string) error {
	info, err := m.GetDeployedInfo(appName)
	if err != nil {
		return fmt.Errorf("reading deploy info: %w", err)
	}

	meta, ok := m.registry.Get(appName)
	if !ok {
		return fmt.Errorf("unknown app template: %s", appName)
	}

	// Refresh routing values from current config
	if m.cfg.Routing.Enabled && m.cfg.Routing.HTTPS.Enabled {
		info.Values["https_enabled"] = "true"
		info.Values["ca_cert_path"] = filepath.Join(m.cfg.DataPath("traefik"), "certs", "ca.crt")
	} else {
		info.Values["https_enabled"] = "false"
		info.Values["ca_cert_path"] = ""
	}
	if m.cfg.Routing.Enabled && info.Routing != nil && info.Routing.Enabled && len(info.Routing.Domains) > 0 {
		scheme := "http"
		if m.cfg.Routing.HTTPS.Enabled {
			scheme = "https"
		}
		info.Values["routing_domain"] = info.Routing.Domains[0]
		info.Values["routing_url"] = fmt.Sprintf("%s://%s", scheme, info.Routing.Domains[0])
	}
	if info.Values["routing_domain"] == "" {
		info.Values["routing_domain"] = appName + "-" + m.cfg.Hostname + "." + m.cfg.Network.Domain
	}
	if info.Values["routing_url"] == "" {
		scheme := "http"
		if m.cfg.Routing.HTTPS.Enabled {
			scheme = "https"
		}
		info.Values["routing_url"] = fmt.Sprintf("%s://%s", scheme, info.Values["routing_domain"])
	}

	// Regenerate CA bundle if HTTPS is enabled
	if m.cfg.Routing.Enabled && m.cfg.Routing.HTTPS.Enabled && info.Values["ca_cert_path"] != "" {
		dataDir := m.cfg.DataPath(appName)
		if err := generateCABundle(info.Values["ca_cert_path"], dataDir); err != nil {
			slog.Warn("Failed to generate CA bundle", "app", appName, "error", err)
		}
	}

	// Re-render templates
	rendered, err := m.renderer.RenderAllFiles(appName, info.Values)
	if err != nil {
		return fmt.Errorf("rendering templates: %w", err)
	}

	// Inject routing labels if enabled
	if m.cfg.Routing.Enabled && m.cfg.Routing.Provider == "traefik" && info.Routing != nil && info.Routing.Enabled {
		labeler := NewRoutingLabeler(m.cfg)
		if compose, ok := rendered["docker-compose.yml"]; ok {
			modified, err := labeler.InjectLabels(compose, appName, info.Routing)
			if err != nil {
				return fmt.Errorf("injecting labels: %w", err)
			}
			rendered["docker-compose.yml"] = modified
		}
	}

	// Write updated compose file
	appDir := m.cfg.AppDir(appName)
	if compose, ok := rendered["docker-compose.yml"]; ok {
		composePath := appDir + "/docker-compose.yml"
		if err := writeFile(composePath, []byte(compose)); err != nil {
			return fmt.Errorf("writing docker-compose.yml: %w", err)
		}
	}

	// Apply changes
	if meta.RequiresBuild {
		_, err = m.compose.UpWithBuild(appDir)
	} else {
		_, err = m.compose.Up(appDir)
	}
	if err != nil {
		return fmt.Errorf("restarting containers: %w", err)
	}

	return nil
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

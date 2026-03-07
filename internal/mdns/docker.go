package mdns

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jdillenberger/homelabctl/internal/exec"
)

var (
	routerRuleRE = regexp.MustCompile(`^traefik\.http\.routers\..+\.rule$`)
	hostExtractRE = regexp.MustCompile("Host\\(`([^`]+)`\\)")
)

// DiscoverTraefikDomains queries running Docker containers for Traefik router
// labels and returns the set of .local domains found.
func DiscoverTraefikDomains(runner *exec.Runner, runtime string) (map[string]bool, error) {
	// Get container IDs with traefik.enable=true
	result, err := runner.Run(runtime, "ps", "-q", "--filter", "label=traefik.enable=true")
	if err != nil {
		return nil, fmt.Errorf("listing traefik containers: %w", err)
	}

	ids := strings.Fields(strings.TrimSpace(result.Stdout))
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}

	// Inspect labels for all containers in one call
	args := append([]string{"inspect", "--format", "{{json .Config.Labels}}"}, ids...)
	result, err = runner.Run(runtime, args...)
	if err != nil {
		return nil, fmt.Errorf("inspecting containers: %w", err)
	}

	domains := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var labels map[string]string
		if err := json.Unmarshal([]byte(line), &labels); err != nil {
			continue
		}

		for key, value := range labels {
			if !isTraefikRouterRule(key) {
				continue
			}
			for _, host := range ExtractHosts(value) {
				if strings.HasSuffix(host, ".local") {
					domains[host] = true
				}
			}
		}
	}

	return domains, nil
}

// ExtractHosts parses Host(`...`) expressions from a Traefik router rule string.
func ExtractHosts(rule string) []string {
	matches := hostExtractRE.FindAllStringSubmatch(rule, -1)
	var hosts []string
	for _, m := range matches {
		hosts = append(hosts, m[1])
	}
	return hosts
}

// isTraefikRouterRule returns true if the label key matches traefik.http.routers.*.rule.
func isTraefikRouterRule(key string) bool {
	return routerRuleRE.MatchString(key)
}

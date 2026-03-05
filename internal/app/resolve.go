package app

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ImageRef represents a parsed container image reference.
type ImageRef struct {
	Registry  string // e.g. "ghcr.io", "docker.io"
	Namespace string // e.g. "immich-app"
	Repo      string // e.g. "immich-server"
	Tag       string // e.g. "release", "latest"
}

// String returns the full image reference.
func (r ImageRef) String() string {
	if r.Registry == "docker.io" && r.Namespace == "library" {
		return fmt.Sprintf("%s:%s", r.Repo, r.Tag)
	}
	if r.Registry == "docker.io" {
		return fmt.Sprintf("%s/%s:%s", r.Namespace, r.Repo, r.Tag)
	}
	return fmt.Sprintf("%s/%s/%s:%s", r.Registry, r.Namespace, r.Repo, r.Tag)
}

// FullRepo returns registry/namespace/repo for API calls.
func (r ImageRef) FullRepo() string {
	return r.Namespace + "/" + r.Repo
}

// ParseImageRef parses a Docker image reference into its components.
func ParseImageRef(image string) (ImageRef, error) {
	ref := ImageRef{}

	// Split tag
	parts := strings.SplitN(image, ":", 2)
	if len(parts) == 2 {
		ref.Tag = parts[1]
	} else {
		ref.Tag = "latest"
	}

	path := parts[0]
	segments := strings.Split(path, "/")

	switch len(segments) {
	case 1:
		// e.g. "nginx" -> docker.io/library/nginx
		ref.Registry = "docker.io"
		ref.Namespace = "library"
		ref.Repo = segments[0]
	case 2:
		// e.g. "gitea/gitea" -> docker.io/gitea/gitea
		ref.Registry = "docker.io"
		ref.Namespace = segments[0]
		ref.Repo = segments[1]
	case 3:
		// e.g. "ghcr.io/immich-app/immich-server"
		ref.Registry = segments[0]
		ref.Namespace = segments[1]
		ref.Repo = segments[2]
	default:
		return ref, fmt.Errorf("unsupported image reference format: %s", image)
	}

	return ref, nil
}

// floatingTags are tags that don't pin to a specific version.
var floatingTags = map[string]bool{
	"latest":  true,
	"release": true,
	"stable":  true,
	"edge":    true,
	"nightly": true,
	"main":    true,
	"master":  true,
}

// IsFloating returns true if the tag is a floating (non-pinned) tag.
func (r ImageRef) IsFloating() bool {
	return floatingTags[r.Tag]
}

// ImageResolver queries container registries to resolve floating tags.
type ImageResolver struct {
	client *http.Client
}

// NewImageResolver creates a new resolver with a default HTTP client.
func NewImageResolver() *ImageResolver {
	return &ImageResolver{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// tokenResponse holds the authentication token from a registry.
type tokenResponse struct {
	Token string `json:"token"`
}

// registryAPIBase returns the API URL for a given registry.
func registryAPIBase(registry string) string {
	switch registry {
	case "docker.io":
		return "https://registry-1.docker.io"
	default:
		return "https://" + registry
	}
}

// tokenServiceURL returns the token service URL for a given registry.
func tokenServiceURL(registry string) string {
	switch registry {
	case "docker.io":
		return "https://auth.docker.io/token"
	case "ghcr.io":
		return "https://ghcr.io/token"
	case "lscr.io":
		return "https://lscr.io/token"
	case "quay.io":
		return "" // quay uses different auth, but public repos don't need tokens
	default:
		return ""
	}
}

// getToken obtains a bearer token for the given registry and repository scope.
func (r *ImageResolver) getToken(registry, repo string) (string, error) {
	tokenURL := tokenServiceURL(registry)
	if tokenURL == "" {
		return "", nil // no auth needed
	}

	service := registry
	if registry == "docker.io" {
		service = "registry.docker.io"
	}

	url := fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", tokenURL, service, repo)
	resp, err := r.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d", resp.StatusCode)
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decoding token: %w", err)
	}
	return tok.Token, nil
}

// GetDigest fetches the manifest digest for a given image reference.
func (r *ImageResolver) GetDigest(ref ImageRef) (string, error) {
	token, err := r.getToken(ref.Registry, ref.FullRepo())
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registryAPIBase(ref.Registry), ref.FullRepo(), ref.Tag)
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("manifest request for %s returned %d: %s", ref.String(), resp.StatusCode, string(body))
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest returned for %s", ref.String())
	}
	return digest, nil
}

// semverRe matches semantic version tags like v1.2.3, 1.2.3, v1.2.3-beta.
var semverRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(-[a-zA-Z0-9.]+)?$`)

// ListTags fetches all tags for the given image reference's repository.
func (r *ImageResolver) ListTags(ref ImageRef) ([]string, error) {
	token, err := r.getToken(ref.Registry, ref.FullRepo())
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v2/%s/tags/list", registryAPIBase(ref.Registry), ref.FullRepo())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tag list for %s returned %d", ref.FullRepo(), resp.StatusCode)
	}

	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding tags: %w", err)
	}

	return result.Tags, nil
}

// ResolveResult holds the result of resolving a floating tag.
type ResolveResult struct {
	Image         string // original image string
	FloatingTag   string // e.g. "latest"
	Digest        string // digest of the floating tag
	PinnedTag     string // highest semver tag with same digest (if found)
	PinnedImage   string // full image string with pinned tag
	TemplateFile  string // source template file path
}

// ResolveFloatingTag resolves a floating tag to the highest semver tag with the same digest.
func (r *ImageResolver) ResolveFloatingTag(ref ImageRef) (*ResolveResult, error) {
	result := &ResolveResult{
		Image:       ref.String(),
		FloatingTag: ref.Tag,
	}

	// Get digest of the floating tag
	digest, err := r.GetDigest(ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref.String(), err)
	}
	result.Digest = digest

	// List all tags
	tags, err := r.ListTags(ref)
	if err != nil {
		return nil, fmt.Errorf("listing tags for %s: %w", ref.String(), err)
	}

	// Filter to semver tags and sort descending
	var semverTags []string
	for _, tag := range tags {
		if semverRe.MatchString(tag) && !floatingTags[tag] {
			semverTags = append(semverTags, tag)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(semverTags)))

	// Find the highest semver tag with the same digest
	for _, tag := range semverTags {
		candidate := ref
		candidate.Tag = tag
		tagDigest, err := r.GetDigest(candidate)
		if err != nil {
			continue
		}
		if tagDigest == digest {
			result.PinnedTag = tag
			candidate.Tag = tag
			result.PinnedImage = candidate.String()
			break
		}
	}

	return result, nil
}

// FloatingTagEntry represents a floating tag found in a template.
type FloatingTagEntry struct {
	AppName  string
	FilePath string
	Image    string
	Ref      ImageRef
}

// ScanFloatingTags scans all templates in the registry for floating image tags.
func ScanFloatingTags(tmplFS fs.FS) ([]FloatingTagEntry, error) {
	imageRe := regexp.MustCompile(`image:\s*(.+)`)
	templateRe := regexp.MustCompile(`\{\{`)

	var entries []FloatingTagEntry

	err := fs.WalkDir(tmplFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		if !strings.HasSuffix(path, "docker-compose.yml.tmpl") {
			return nil
		}

		data, err := fs.ReadFile(tmplFS, path)
		if err != nil {
			return err
		}

		matches := imageRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			imageStr := strings.TrimSpace(m[1])

			// Skip dynamic template tags like {{index . "desktop_env"}}
			if templateRe.MatchString(imageStr) {
				continue
			}

			ref, err := ParseImageRef(imageStr)
			if err != nil {
				continue
			}

			if ref.IsFloating() {
				parts := strings.SplitN(path, "/", 2)
				appName := parts[0]
				entries = append(entries, FloatingTagEntry{
					AppName:  appName,
					FilePath: path,
					Image:    imageStr,
					Ref:      ref,
				})
			}
		}

		return nil
	})

	return entries, err
}

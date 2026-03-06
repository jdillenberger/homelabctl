package app

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strconv"
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

// isRegistryHost returns true if the segment looks like a registry hostname
// (contains a dot or a colon for port-based registries like registry.local:5000).
func isRegistryHost(s string) bool {
	return strings.ContainsAny(s, ".:")
}

// ParseImageRef parses a Docker image reference into its components.
// Supports formats:
//   - "nginx" -> docker.io/library/nginx:latest
//   - "gitea/gitea:1.2.3" -> docker.io/gitea/gitea:1.2.3
//   - "ghcr.io/org/repo:tag" -> ghcr.io/org/repo:tag
//   - "registry.local:5000/org/repo:tag" -> registry.local:5000/org/repo:tag
func ParseImageRef(image string) (ImageRef, error) {
	ref := ImageRef{}

	// Split off the tag. The tag is after the last ":" but only if it comes
	// after the last "/". This avoids confusing "registry:5000/repo" port
	// with a tag.
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		ref.Tag = image[lastColon+1:]
		image = image[:lastColon]
	} else {
		ref.Tag = "latest"
	}

	segments := strings.Split(image, "/")

	switch len(segments) {
	case 1:
		// e.g. "nginx" -> docker.io/library/nginx
		ref.Registry = "docker.io"
		ref.Namespace = "library"
		ref.Repo = segments[0]
	case 2:
		if isRegistryHost(segments[0]) {
			// e.g. "registry.local:5000/repo" -> registry.local:5000/library/repo
			ref.Registry = segments[0]
			ref.Namespace = "library"
			ref.Repo = segments[1]
		} else {
			// e.g. "gitea/gitea" -> docker.io/gitea/gitea
			ref.Registry = "docker.io"
			ref.Namespace = segments[0]
			ref.Repo = segments[1]
		}
	case 3:
		// e.g. "ghcr.io/immich-app/immich-server"
		ref.Registry = segments[0]
		ref.Namespace = segments[1]
		ref.Repo = segments[2]
	default:
		if len(segments) > 3 && isRegistryHost(segments[0]) {
			// e.g. "ghcr.io/org/suborg/repo" -> registry=ghcr.io, namespace=org/suborg, repo=repo
			ref.Registry = segments[0]
			ref.Repo = segments[len(segments)-1]
			ref.Namespace = strings.Join(segments[1:len(segments)-1], "/")
		} else {
			return ref, fmt.Errorf("unsupported image reference format: %s", image)
		}
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
	req, err := http.NewRequest("HEAD", url, http.NoBody)
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

// SemVer represents a parsed semantic version.
type SemVer struct {
	Major int
	Minor int
	Patch int
	Pre   string // pre-release suffix, e.g. "beta", "rc.1"
}

// String returns the normalized version string (without v prefix).
func (v SemVer) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// ParseSemver parses a version tag into a SemVer struct.
// Accepts formats: "1.2.3", "v1.2.3", "1.2.3-beta", "v1.2.3-rc.1".
func ParseSemver(tag string) (SemVer, error) {
	m := semverRe.FindStringSubmatch(tag)
	if m == nil {
		return SemVer{}, fmt.Errorf("not a semver tag: %s", tag)
	}

	parts := strings.Split(m[1], ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	pre := ""
	if m[2] != "" {
		pre = m[2][1:] // strip leading "-"
	}

	return SemVer{Major: major, Minor: minor, Patch: patch, Pre: pre}, nil
}

// CompareSemver compares two SemVer values.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Pre-release versions are considered less than the release version.
func CompareSemver(a, b SemVer) int {
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}
	// Both have no pre-release: equal
	if a.Pre == "" && b.Pre == "" {
		return 0
	}
	// Pre-release < release
	if a.Pre != "" && b.Pre == "" {
		return -1
	}
	if a.Pre == "" && b.Pre != "" {
		return 1
	}
	// Both have pre-release: lexicographic
	if a.Pre < b.Pre {
		return -1
	}
	if a.Pre > b.Pre {
		return 1
	}
	return 0
}

// UpgradeType returns "patch", "minor", or "major" based on the difference between two versions.
func UpgradeType(from, to SemVer) string {
	if to.Major != from.Major {
		return "major"
	}
	if to.Minor != from.Minor {
		return "minor"
	}
	return "patch"
}

// VersionUpdate describes an available version upgrade.
type VersionUpdate struct {
	CurrentTag string `json:"current_tag"`
	NewTag     string `json:"new_tag"`
	Type       string `json:"type"` // "patch", "minor", "major"
}

// FindNewerVersions queries the registry for the given image ref, filters to semver tags,
// and returns the latest available update for each upgrade type (patch/minor/major).
func (r *ImageResolver) FindNewerVersions(ref ImageRef) ([]VersionUpdate, error) {
	currentVer, err := ParseSemver(ref.Tag)
	if err != nil {
		return nil, fmt.Errorf("current tag %q is not semver: %w", ref.Tag, err)
	}

	tags, err := r.ListTags(ref)
	if err != nil {
		return nil, err
	}

	// Track the latest version for each upgrade type
	best := map[string]SemVer{}    // type -> best version
	bestTag := map[string]string{} // type -> original tag string

	for _, tag := range tags {
		v, err := ParseSemver(tag)
		if err != nil {
			continue
		}
		// Skip pre-release versions
		if v.Pre != "" {
			continue
		}
		// Must be newer than current
		if CompareSemver(v, currentVer) <= 0 {
			continue
		}

		utype := UpgradeType(currentVer, v)
		if prev, exists := best[utype]; !exists || CompareSemver(v, prev) > 0 {
			best[utype] = v
			bestTag[utype] = tag
		}
	}

	var updates []VersionUpdate
	for _, utype := range []string{"patch", "minor", "major"} {
		if tag, ok := bestTag[utype]; ok {
			updates = append(updates, VersionUpdate{
				CurrentTag: ref.Tag,
				NewTag:     tag,
				Type:       utype,
			})
		}
	}
	return updates, nil
}

// ImageEntry represents any image found in a template (not just floating tags).
type ImageEntry struct {
	AppName  string
	FilePath string
	Image    string
	Ref      ImageRef
}

// imageLineRe matches YAML "image:" keys with proper indentation, skipping comments.
// Requires the line to start with optional whitespace, then "image:", avoiding
// false matches on commented-out lines or non-image keys.
var imageLineRe = regexp.MustCompile(`(?m)^\s+image:\s*(.+?)(?:\s*#.*)?$`)
var templateRe = regexp.MustCompile(`\{\{`)

// scanImages is the shared implementation for scanning image references from template files.
// When floatingOnly is true, only floating-tagged images are returned.
func scanImages(tmplFS fs.FS, floatingOnly bool) ([]ImageEntry, error) {
	var entries []ImageEntry

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

		matches := imageLineRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			imageStr := strings.TrimSpace(m[1])

			if templateRe.MatchString(imageStr) {
				continue
			}

			ref, err := ParseImageRef(imageStr)
			if err != nil {
				continue
			}

			if floatingOnly && !ref.IsFloating() {
				continue
			}

			parts := strings.SplitN(path, "/", 2)
			appName := parts[0]
			entries = append(entries, ImageEntry{
				AppName:  appName,
				FilePath: path,
				Image:    imageStr,
				Ref:      ref,
			})
		}

		return nil
	})

	return entries, err
}

// ScanAllImages scans all templates for image references, returning all images
// (not just floating tags).
func ScanAllImages(tmplFS fs.FS) ([]ImageEntry, error) {
	return scanImages(tmplFS, false)
}

// ScanDeployedImages parses image references from a deployed docker-compose.yml file.
func ScanDeployedImages(data []byte) ([]ImageRef, error) {
	matches := imageLineRe.FindAllStringSubmatch(string(data), -1)

	var refs []ImageRef
	for _, m := range matches {
		imageStr := strings.TrimSpace(m[1])
		ref, err := ParseImageRef(imageStr)
		if err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// ListTags fetches all tags for the given image reference's repository.
// Follows pagination via the Link header as per the Docker Registry HTTP API v2.
func (r *ImageResolver) ListTags(ref ImageRef) ([]string, error) {
	token, err := r.getToken(ref.Registry, ref.FullRepo())
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("%s/v2/%s/tags/list", registryAPIBase(ref.Registry), ref.FullRepo())
	var allTags []string
	nextURL := baseURL

	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, http.NoBody)
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

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("tag list for %s returned %d", ref.FullRepo(), resp.StatusCode)
		}

		var result struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding tags: %w", err)
		}
		resp.Body.Close()

		allTags = append(allTags, result.Tags...)

		// Follow pagination via Link header: <url>; rel="next"
		nextURL = ""
		if link := resp.Header.Get("Link"); link != "" {
			nextURL = parseLinkNext(link, baseURL)
		}
	}

	return allTags, nil
}

// parseLinkNext extracts the "next" URL from a Link header value.
// Format: </v2/repo/tags/list?n=100&last=tag>; rel="next"
func parseLinkNext(link, baseURL string) string {
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		ref := part[start+1 : end]
		// The link may be relative (just a path) or absolute
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			return ref
		}
		// Relative URL: resolve against the base
		base := baseURL
		if idx := strings.Index(base, "/v2/"); idx >= 0 {
			base = base[:idx]
		}
		return base + ref
	}
	return ""
}

// ResolveResult holds the result of resolving a floating tag.
type ResolveResult struct {
	Image        string // original image string
	FloatingTag  string // e.g. "latest"
	Digest       string // digest of the floating tag
	PinnedTag    string // highest semver tag with same digest (if found)
	PinnedImage  string // full image string with pinned tag
	TemplateFile string // source template file path
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

	// Filter to semver tags and sort descending by actual semver comparison
	type taggedVersion struct {
		tag string
		ver SemVer
	}
	var semverTagged []taggedVersion
	for _, tag := range tags {
		if floatingTags[tag] {
			continue
		}
		v, err := ParseSemver(tag)
		if err != nil {
			continue
		}
		semverTagged = append(semverTagged, taggedVersion{tag: tag, ver: v})
	}
	sort.Slice(semverTagged, func(i, j int) bool {
		return CompareSemver(semverTagged[i].ver, semverTagged[j].ver) > 0 // descending
	})

	// Find the highest semver tag with the same digest
	for _, tv := range semverTagged {
		candidate := ref
		candidate.Tag = tv.tag
		tagDigest, err := r.GetDigest(candidate)
		if err != nil {
			continue
		}
		if tagDigest == digest {
			result.PinnedTag = tv.tag
			candidate.Tag = tv.tag
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
	images, err := scanImages(tmplFS, true)
	if err != nil {
		return nil, err
	}
	entries := make([]FloatingTagEntry, len(images))
	for i, img := range images {
		entries[i] = FloatingTagEntry(img)
	}
	return entries, nil
}

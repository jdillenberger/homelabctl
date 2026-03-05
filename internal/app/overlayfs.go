package app

import (
	"io/fs"
	"os"
	"sort"
	"strings"
)

// OverlayFS combines two filesystems where the upper layer takes precedence
// at the top-level directory boundary. If a template directory exists in
// the upper FS, all files for that template come from upper. Otherwise,
// they come from lower. This enables local template overrides without
// modifying the embedded defaults.
type OverlayFS struct {
	upper     fs.FS
	lower     fs.FS
	upperDirs map[string]bool
	lowerDirs map[string]bool
}

// Verify interface compliance.
var (
	_ fs.FS         = (*OverlayFS)(nil)
	_ fs.ReadDirFS  = (*OverlayFS)(nil)
	_ fs.ReadFileFS = (*OverlayFS)(nil)
)

// NewOverlayFS creates an OverlayFS. upper takes precedence over lower
// at the template directory level.
func NewOverlayFS(lower, upper fs.FS) *OverlayFS {
	o := &OverlayFS{
		upper:     upper,
		lower:     lower,
		upperDirs: make(map[string]bool),
		lowerDirs: make(map[string]bool),
	}

	if entries, err := fs.ReadDir(lower, "."); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				o.lowerDirs[e.Name()] = true
			}
		}
	}

	if upper != nil {
		if entries, err := fs.ReadDir(upper, "."); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					o.upperDirs[e.Name()] = true
				}
			}
		}
	}

	return o
}

func topDir(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return name
}

// fsFor returns the FS that owns the given path.
func (o *OverlayFS) fsFor(name string) fs.FS {
	if o.upper != nil && o.upperDirs[topDir(name)] {
		return o.upper
	}
	return o.lower
}

// Open implements fs.FS.
func (o *OverlayFS) Open(name string) (fs.File, error) {
	return o.fsFor(name).Open(name)
}

// ReadFile implements fs.ReadFileFS.
func (o *OverlayFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(o.fsFor(name), name)
}

// ReadDir implements fs.ReadDirFS.
func (o *OverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return o.mergedRootEntries()
	}
	return fs.ReadDir(o.fsFor(name), name)
}

// mergedRootEntries returns directory entries from both layers, with upper
// entries taking precedence over lower entries of the same name.
func (o *OverlayFS) mergedRootEntries() ([]fs.DirEntry, error) {
	lowerEntries, err := fs.ReadDir(o.lower, ".")
	if err != nil {
		return nil, err
	}

	if o.upper == nil {
		return lowerEntries, nil
	}

	seen := make(map[string]bool)
	var merged []fs.DirEntry

	if upperEntries, err := fs.ReadDir(o.upper, "."); err == nil {
		for _, e := range upperEntries {
			seen[e.Name()] = true
			merged = append(merged, e)
		}
	}

	for _, e := range lowerEntries {
		if !seen[e.Name()] {
			merged = append(merged, e)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Name() < merged[j].Name()
	})

	return merged, nil
}

// Source returns where a template comes from:
//   - "built-in" — only exists in the embedded (lower) FS
//   - "local" — only exists in the local (upper) FS
//   - "override" — exists in both, local version takes precedence
func (o *OverlayFS) Source(templateName string) string {
	inUpper := o.upper != nil && o.upperDirs[templateName]
	inLower := o.lowerDirs[templateName]

	switch {
	case inUpper && inLower:
		return "override"
	case inUpper:
		return "local"
	default:
		return "built-in"
	}
}

// BuildTemplateFS creates the template filesystem by overlaying the local
// templates directory on top of the embedded FS. The local directory is
// created automatically if it does not exist.
func BuildTemplateFS(embeddedFS fs.FS, localDir string) fs.FS {
	if localDir == "" {
		return embeddedFS
	}

	// Auto-create the local templates directory.
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return embeddedFS
	}

	info, err := os.Stat(localDir)
	if err != nil || !info.IsDir() {
		return embeddedFS
	}

	return NewOverlayFS(embeddedFS, os.DirFS(localDir))
}

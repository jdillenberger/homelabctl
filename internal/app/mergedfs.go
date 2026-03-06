package app

import (
	"io/fs"
	"os"
	"sort"
	"time"
)

// MergedFS combines multiple directory-based filesystems into a single fs.FS.
// The first layer in the list wins on template name conflicts.
type MergedFS struct {
	layers []fs.FS
	dirs   map[string]int // template name → layer index
}

// Verify interface compliance.
var (
	_ fs.FS         = (*MergedFS)(nil)
	_ fs.ReadDirFS  = (*MergedFS)(nil)
	_ fs.ReadFileFS = (*MergedFS)(nil)
)

// NewMergedFS creates a MergedFS from a list of directory paths.
// Earlier directories take precedence on name conflicts.
func NewMergedFS(dirs []string) *MergedFS {
	m := &MergedFS{
		dirs: make(map[string]int),
	}

	for i, dir := range dirs {
		dirFS := os.DirFS(dir)
		m.layers = append(m.layers, dirFS)

		entries, err := fs.ReadDir(dirFS, ".")
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, exists := m.dirs[e.Name()]; !exists {
				m.dirs[e.Name()] = i
			}
		}
	}

	return m
}

// RepoIndex returns which layer owns a template (-1 if not found).
func (m *MergedFS) RepoIndex(templateName string) int {
	idx, ok := m.dirs[templateName]
	if !ok {
		return -1
	}
	return idx
}

// Open implements fs.FS.
func (m *MergedFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &mergedDir{m: m}, nil
	}
	top := topDir(name)
	idx, ok := m.dirs[top]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return m.layers[idx].Open(name)
}

// ReadFile implements fs.ReadFileFS.
func (m *MergedFS) ReadFile(name string) ([]byte, error) {
	top := topDir(name)
	idx, ok := m.dirs[top]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadFile(m.layers[idx], name)
}

// ReadDir implements fs.ReadDirFS.
func (m *MergedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return m.rootEntries()
	}
	top := topDir(name)
	idx, ok := m.dirs[top]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadDir(m.layers[idx], name)
}

// rootEntries returns sorted directory entries for all merged template dirs.
func (m *MergedFS) rootEntries() ([]fs.DirEntry, error) {
	entries := make([]fs.DirEntry, 0, len(m.dirs))
	for name, idx := range m.dirs {
		info, err := fs.Stat(m.layers[idx], name)
		if err != nil {
			continue
		}
		entries = append(entries, fs.FileInfoToDirEntry(info))
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

// mergedDir implements fs.File for the root directory of a MergedFS.
type mergedDir struct {
	m *MergedFS
}

func (d *mergedDir) Stat() (fs.FileInfo, error) {
	return &dirInfo{name: "."}, nil
}

func (d *mergedDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid}
}

func (d *mergedDir) Close() error { return nil }

// dirInfo is a minimal fs.FileInfo for a synthetic directory.
type dirInfo struct {
	name string
}

func (di *dirInfo) Name() string      { return di.name }
func (di *dirInfo) Size() int64       { return 0 }
func (di *dirInfo) Mode() fs.FileMode { return fs.ModeDir | 0o755 }
func (di *dirInfo) ModTime() time.Time { return time.Time{} }
func (di *dirInfo) IsDir() bool        { return true }
func (di *dirInfo) Sys() any           { return nil }

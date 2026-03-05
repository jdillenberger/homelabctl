package backup

import (
	"fmt"
	"strings"

	"github.com/jdillenberger/homelabctl/internal/exec"
)

// Archive represents a single borg archive entry.
type Archive struct {
	Name string
	Date string
}

// Borg wraps the borgmatic CLI.
type Borg struct {
	runner *exec.Runner
}

// NewBorg creates a new Borg wrapper.
func NewBorg(runner *exec.Runner) *Borg {
	return &Borg{runner: runner}
}

// Init initializes a borg repository via borgmatic.
func (b *Borg) Init(repo string) (*exec.Result, error) {
	return b.runner.Run("borgmatic", "init", "--encryption", "repokey")
}

// Create runs a borgmatic backup using the given config file.
func (b *Borg) Create(configFile string) (*exec.Result, error) {
	return b.runner.Run("borgmatic", "create", "-c", configFile)
}

// List lists archives using the given config file and parses the output.
func (b *Borg) List(configFile string) ([]Archive, error) {
	result, err := b.runner.Run("borgmatic", "list", "-c", configFile)
	if err != nil {
		return nil, fmt.Errorf("listing archives: %w", err)
	}
	return parseArchiveList(result.Stdout), nil
}

// Restore extracts an archive using the given config file.
// If archive is empty, borgmatic will use the latest archive.
func (b *Borg) Restore(configFile, archive string) (*exec.Result, error) {
	args := []string{"extract", "-c", configFile}
	if archive != "" {
		args = append(args, "--archive", archive)
	}
	return b.runner.Run("borgmatic", args...)
}

// parseArchiveList parses borgmatic list output into Archive structs.
// Each non-empty line is expected to contain at least an archive name.
func parseArchiveList(output string) []Archive {
	var archives []Archive
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		a := Archive{Name: fields[0]}
		if len(fields) >= 3 {
			a.Date = fields[1] + " " + fields[2]
		}
		archives = append(archives, a)
	}
	return archives
}

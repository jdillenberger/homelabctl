package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jdillenberger/homelabctl/internal/exec"
)

const (
	composeDefaultTimeout = 5 * time.Minute
	composeLongTimeout    = 10 * time.Minute
)

// Compose wraps the docker compose CLI.
type Compose struct {
	runner  *exec.Runner
	command string // e.g. "docker compose"
}

// NewCompose creates a new Compose wrapper.
func NewCompose(runner *exec.Runner, composeCommand string) *Compose {
	return &Compose{
		runner:  runner,
		command: composeCommand,
	}
}

func (c *Compose) cmdParts() (bin string, args []string) {
	parts := strings.Fields(c.command)
	if len(parts) == 0 {
		return "docker", []string{"compose"}
	}
	return parts[0], parts[1:]
}

func (c *Compose) run(projectDir string, timeout time.Duration, args ...string) (*exec.Result, error) {
	bin, baseArgs := c.cmdParts()
	fullArgs := make([]string, 0, len(baseArgs)+len(args)+2)
	fullArgs = append(fullArgs, baseArgs...)
	fullArgs = append(fullArgs, "-f", projectDir+"/docker-compose.yml")
	fullArgs = append(fullArgs, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.runner.RunWithContext(ctx, bin, fullArgs...)
}

// Up runs docker compose up -d.
func (c *Compose) Up(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeLongTimeout, "up", "-d", "--remove-orphans")
}

// Down runs docker compose down.
func (c *Compose) Down(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "down")
}

// Start runs docker compose start.
func (c *Compose) Start(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "start")
}

// Stop runs docker compose stop.
func (c *Compose) Stop(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "stop")
}

// Restart runs docker compose restart.
func (c *Compose) Restart(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "restart")
}

// PS returns the status of containers in the project.
func (c *Compose) PS(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "ps", "--format", "table")
}

// PSJson returns the status of containers in JSON format for structured parsing.
func (c *Compose) PSJson(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeDefaultTimeout, "ps", "--format", "json")
}

// Logs streams logs to the given writer.
func (c *Compose) Logs(projectDir string, w io.Writer, follow bool, lines int) error {
	bin, baseArgs := c.cmdParts()
	fullArgs := make([]string, 0, len(baseArgs)+7)
	fullArgs = append(fullArgs, baseArgs...)
	fullArgs = append(fullArgs, "-f", projectDir+"/docker-compose.yml", "logs", "--no-color")
	if follow {
		fullArgs = append(fullArgs, "-f")
	}
	if lines > 0 {
		fullArgs = append(fullArgs, "-n", fmt.Sprintf("%d", lines))
	}
	return c.runner.RunStream(w, bin, fullArgs...)
}

// Build runs docker compose build.
func (c *Compose) Build(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeLongTimeout, "build")
}

// UpWithBuild runs docker compose up -d --build.
func (c *Compose) UpWithBuild(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeLongTimeout, "up", "-d", "--build", "--remove-orphans")
}

// Pull pulls the latest images.
func (c *Compose) Pull(projectDir string) (*exec.Result, error) {
	return c.run(projectDir, composeLongTimeout, "pull")
}

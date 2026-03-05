package exec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner wraps os/exec with logging support.
type Runner struct {
	Verbose bool
}

// Result holds command execution results.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run executes a command and returns captured output.
func (r *Runner) Run(name string, args ...string) (*Result, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "exec: %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("command %q exited with code %d: %s", name, result.ExitCode, stderr.String())
	}
	if err != nil {
		return result, fmt.Errorf("command %q failed: %w", name, err)
	}

	return result, nil
}

// RunInteractive runs a command with stdin/stdout/stderr attached.
func (r *Runner) RunInteractive(name string, args ...string) error {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "exec: %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunStream runs a command streaming stdout to the given writer.
func (r *Runner) RunStream(w io.Writer, name string, args ...string) error {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "exec: %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

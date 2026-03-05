package exec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	r := &Runner{Verbose: false}
	result, err := r.Run("echo", "hello")
	if err != nil {
		t.Fatalf("Run(echo hello) error: %v", err)
	}

	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("expected stdout='hello', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode=0, got %d", result.ExitCode)
	}
}

func TestRunFailure(t *testing.T) {
	r := &Runner{Verbose: false}
	result, err := r.Run("false")
	if err == nil {
		t.Fatal("Run(false) should return an error")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero ExitCode for 'false' command")
	}
}

func TestRunNonexistentCommand(t *testing.T) {
	r := &Runner{Verbose: false}
	_, err := r.Run("this-command-does-not-exist-xyz-12345")
	if err == nil {
		t.Fatal("Run() with nonexistent command should return an error")
	}
}

func TestRunCapturesStderr(t *testing.T) {
	r := &Runner{Verbose: false}
	result, err := r.Run("sh", "-c", "echo errout >&2")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(result.Stderr, "errout") {
		t.Errorf("expected stderr to contain 'errout', got %q", result.Stderr)
	}
}

func TestRunVerboseMode(t *testing.T) {
	// Verbose mode writes to stderr but should not affect correctness.
	r := &Runner{Verbose: true}
	result, err := r.Run("echo", "test")
	if err != nil {
		t.Fatalf("Run() with Verbose=true error: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "test" {
		t.Errorf("expected stdout='test', got %q", result.Stdout)
	}
}

func TestRunWithContextSuccess(t *testing.T) {
	r := &Runner{Verbose: false}
	ctx := context.Background()
	result, err := r.RunWithContext(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("RunWithContext() error: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("expected stdout='hello', got %q", result.Stdout)
	}
}

func TestRunWithContextTimeout(t *testing.T) {
	r := &Runner{Verbose: false}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := r.RunWithContext(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("RunWithContext() with expired context should return an error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline error, got: %v", err)
	}
}

func TestRunWithContextCancellation(t *testing.T) {
	r := &Runner{Verbose: false}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	_, err := r.RunWithContext(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("RunWithContext() with cancelled context should return an error")
	}
}

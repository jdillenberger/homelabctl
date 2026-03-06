package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationAppLifecycle tests the full deploy → status → logs → restart → remove cycle.
// Requires Docker to be running. Skip with -short flag or if Docker is unavailable.
func TestIntegrationAppLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found, skipping integration test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not running, skipping integration test")
	}

	binary := buildBinary(t)
	appName := "portainer" // lightweight test app

	// Use a temp directory for config
	tmpDir := t.TempDir()
	appsDir := filepath.Join(tmpDir, "apps")
	dataDir := filepath.Join(tmpDir, "data")
	configFile := filepath.Join(tmpDir, "config.yaml")

	_ = os.WriteFile(configFile, []byte(
		"hostname: test-host\n"+
			"apps_dir: "+appsDir+"\n"+
			"data_dir: "+dataDir+"\n"+
			"network:\n"+
			"  domain: local\n"+
			"  web_port: 9999\n"+
			"docker:\n"+
			"  compose_command: docker compose\n"+
			"  default_network: homelabctl-test-net\n"+
			"mdns:\n"+
			"  enabled: false\n"+
			"backup:\n"+
			"  enabled: false\n",
	), 0o644)

	run := func(args ...string) string {
		fullArgs := append([]string{"--config", configFile}, args...)
		cmd := exec.Command(binary, fullArgs...)
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Command failed: %s %s\nOutput: %s\nError: %v", binary, strings.Join(fullArgs, " "), string(out), err)
		}
		return string(out)
	}

	runExpectSuccess := func(args ...string) string {
		fullArgs := append([]string{"--config", configFile}, args...)
		cmd := exec.Command(binary, fullArgs...)
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Expected success for: %s %s\nOutput: %s\nError: %v", binary, strings.Join(fullArgs, " "), string(out), err)
		}
		return string(out)
	}

	// Cleanup on failure
	t.Cleanup(func() {
		run("apps", "remove", appName)
		_ = exec.Command("docker", "network", "rm", "homelabctl-test-net").Run()
	})

	// Deploy (pass default values to avoid interactive wizard)
	t.Run("deploy", func(t *testing.T) {
		out := runExpectSuccess("apps", "deploy", appName, "-y",
			"--set", "http_port=9000", "--set", "https_port=9443")
		if !strings.Contains(out, "deployed successfully") {
			t.Errorf("expected deploy success message, got: %s", out)
		}
	})

	// Status
	t.Run("status", func(t *testing.T) {
		out := runExpectSuccess("apps", "status", appName)
		if out == "" {
			t.Error("expected status output")
		}
	})

	// Logs
	t.Run("logs", func(t *testing.T) {
		_ = run("apps", "logs", appName, "-n", "5")
		// Don't fail on logs — container might not have output yet
	})

	// Restart
	t.Run("restart", func(t *testing.T) {
		runExpectSuccess("apps", "restart", appName)
	})

	// List
	t.Run("list", func(t *testing.T) {
		out := runExpectSuccess("apps", "list")
		if !strings.Contains(out, appName) {
			t.Errorf("expected %s in list output, got: %s", appName, out)
		}
	})

	// List --json
	t.Run("list-json", func(t *testing.T) {
		out := runExpectSuccess("--json", "apps", "list")
		if !strings.Contains(out, appName) {
			t.Errorf("expected %s in JSON list output, got: %s", appName, out)
		}
	})

	// Remove
	t.Run("remove", func(t *testing.T) {
		out := runExpectSuccess("apps", "remove", appName)
		if !strings.Contains(out, "removed") {
			t.Errorf("expected remove message, got: %s", out)
		}
	})
}

func buildBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "homelabctl")
	cmd := exec.Command("go", "build", "-o", binary, "../cmd/homelabctl")
	cmd.Dir = filepath.Dir(".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, string(out))
	}
	return binary
}

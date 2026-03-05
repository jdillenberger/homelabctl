package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	githubRepo   = "jdillenberger/homelabctl"
	githubAPIURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

func init() {
	selfUpdateCmd.Flags().Bool("check", false, "Check for updates without installing")
	rootCmd.AddCommand(selfUpdateCmd)
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update homelabctl to the latest version",
	Long:  "Check for and install the latest release from GitHub.",
	RunE: func(cmd *cobra.Command, args []string) error {
		checkOnly, _ := cmd.Flags().GetBool("check")

		// Fetch latest release info
		resp, err := http.Get(githubAPIURL)
		if err != nil {
			return fmt.Errorf("checking for updates: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var release githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return fmt.Errorf("parsing release info: %w", err)
		}

		latest := strings.TrimPrefix(release.TagName, "v")
		current := strings.TrimPrefix(version, "v")

		if jsonOutput {
			return outputJSON(map[string]string{
				"current_version": current,
				"latest_version":  latest,
				"update_available": fmt.Sprintf("%t", current != latest),
			})
		}

		if current == latest {
			fmt.Printf("Already up to date (version %s).\n", current)
			return nil
		}

		fmt.Printf("Current version: %s\n", current)
		fmt.Printf("Latest version:  %s\n", latest)

		if checkOnly {
			fmt.Println("Update available. Run 'homelabctl self-update' to install.")
			return nil
		}

		// Find the right asset for this OS/arch
		archName := mapArch(runtime.GOARCH)
		assetName := fmt.Sprintf("homelabctl_%s_%s", runtime.GOOS, archName)

		var downloadURL string
		for _, asset := range release.Assets {
			if strings.Contains(asset.Name, assetName) && strings.HasSuffix(asset.Name, ".tar.gz") {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}

		if downloadURL == "" {
			return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
		}

		fmt.Printf("Downloading %s...\n", downloadURL)

		// Download the binary
		dlResp, err := http.Get(downloadURL)
		if err != nil {
			return fmt.Errorf("downloading update: %w", err)
		}
		defer dlResp.Body.Close()

		if dlResp.StatusCode != http.StatusOK {
			return fmt.Errorf("download returned status %d", dlResp.StatusCode)
		}

		// Write to a temp file
		tmpFile, err := os.CreateTemp("", "homelabctl-update-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
			tmpFile.Close()
			return fmt.Errorf("writing update: %w", err)
		}
		tmpFile.Close()

		// Get current binary path
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding current binary: %w", err)
		}

		// Replace current binary
		if err := os.Rename(tmpFile.Name(), execPath); err != nil {
			// If rename fails (cross-device), try copy
			src, err := os.Open(tmpFile.Name())
			if err != nil {
				return fmt.Errorf("opening temp file: %w", err)
			}
			defer src.Close()

			dst, err := os.OpenFile(execPath, os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("opening binary for writing: %w", err)
			}
			defer dst.Close()

			if _, err := io.Copy(dst, src); err != nil {
				return fmt.Errorf("copying update: %w", err)
			}
		}

		if err := os.Chmod(execPath, 0o755); err != nil {
			return fmt.Errorf("setting permissions: %w", err)
		}

		fmt.Printf("Updated to version %s.\n", latest)
		return nil
	},
}

func mapArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv7"
	default:
		return goarch
	}
}

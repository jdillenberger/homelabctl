package cli

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"

	"github.com/jdillenberger/homelabctl/internal/config"
	"github.com/jdillenberger/homelabctl/internal/netutil"
)

func init() {
	rootCmd.AddCommand(caCmd)
	caCmd.AddCommand(caInfoCmd)
	caCmd.AddCommand(caExportCmd)
	caCmd.AddCommand(caTrustCmd)
	caCmd.AddCommand(caQRCmd)
}

var caCmd = &cobra.Command{
	Use:   "ca",
	Short: "Manage the local CA certificate",
	Long:  "View, export, and install the homelabctl local CA certificate used for HTTPS.",
}

var caInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show CA certificate details",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		caCertPath := filepath.Join(cfg.DataPath("traefik"), "certs", "ca.crt")
		cert, err := loadX509Cert(caCertPath)
		if err != nil {
			return fmt.Errorf("CA certificate not found at %s — deploy traefik first", caCertPath)
		}

		fingerprint := sha256.Sum256(cert.Raw)

		if jsonOutput {
			return outputJSON(map[string]string{
				"subject":     cert.Subject.CommonName,
				"issuer":      cert.Issuer.CommonName,
				"not_before":  cert.NotBefore.Format("2006-01-02"),
				"not_after":   cert.NotAfter.Format("2006-01-02"),
				"fingerprint": fmt.Sprintf("%X", fingerprint),
				"path":        caCertPath,
			})
		}

		fmt.Printf("Subject:      %s\n", cert.Subject.CommonName)
		fmt.Printf("Organization: %s\n", cert.Subject.Organization)
		fmt.Printf("Valid from:   %s\n", cert.NotBefore.Format("2006-01-02"))
		fmt.Printf("Valid until:  %s\n", cert.NotAfter.Format("2006-01-02"))
		fmt.Printf("Fingerprint:  %X\n", fingerprint)
		fmt.Printf("Path:         %s\n", caCertPath)

		return nil
	},
}

var caExportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Export the CA certificate to a file",
	Long:  "Copy the CA certificate to the specified path. Defaults to ./homelabctl-ca.crt",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		caCertPath := filepath.Join(cfg.DataPath("traefik"), "certs", "ca.crt")
		if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
			return fmt.Errorf("CA certificate not found at %s — deploy traefik first", caCertPath)
		}

		dest := "homelabctl-ca.crt"
		if len(args) > 0 {
			dest = args[0]
		}

		data, err := os.ReadFile(caCertPath)
		if err != nil {
			return fmt.Errorf("reading CA cert: %w", err)
		}

		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("writing CA cert: %w", err)
		}

		fmt.Printf("CA certificate exported to %s\n", dest)
		return nil
	},
}

var caTrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Install the CA certificate into the system trust store",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		caCertPath := filepath.Join(cfg.DataPath("traefik"), "certs", "ca.crt")
		if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
			return fmt.Errorf("CA certificate not found at %s — deploy traefik first", caCertPath)
		}

		switch runtime.GOOS {
		case "linux":
			return trustLinux(caCertPath)
		case "darwin":
			return trustDarwin(caCertPath)
		default:
			return fmt.Errorf("automatic trust installation not supported on %s — use 'homelabctl ca export' and install manually", runtime.GOOS)
		}
	},
}

var caQRCmd = &cobra.Command{
	Use:   "qr",
	Short: "Print a QR code for the CA trust page",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		ip := netutil.DetectLocalIP()
		if ip == "" {
			return fmt.Errorf("could not detect local IP address")
		}
		url := fmt.Sprintf("http://%s:%d/ca", ip, cfg.Network.WebPort)

		qr, err := qrcode.New(url, qrcode.Medium)
		if err != nil {
			return fmt.Errorf("generating QR code: %w", err)
		}

		fmt.Println(qr.ToSmallString(false))
		fmt.Printf("URL: %s\n", url)
		return nil
	},
}

func trustLinux(caCertPath string) error {
	dest := "/usr/local/share/ca-certificates/homelabctl-ca.crt"

	// Copy cert
	data, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("reading CA cert: %w", err)
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied — run with sudo: sudo homelabctl ca trust")
		}
		return fmt.Errorf("writing CA cert: %w", err)
	}

	// Update trust store
	out, err := exec.Command("update-ca-certificates").CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-ca-certificates failed: %s", out)
	}

	fmt.Println("CA certificate installed into system trust store.")
	return nil
}

func trustDarwin(caCertPath string) error {
	out, err := exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain", caCertPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-trusted-cert failed: %s", out)
	}

	fmt.Println("CA certificate installed into system trust store.")
	return nil
}

func loadX509Cert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

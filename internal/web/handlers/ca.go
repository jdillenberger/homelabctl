package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/jdillenberger/homelabctl/internal/netutil"
)

// CAPageData holds data for the CA trust page template.
type CAPageData struct {
	BasePage
	Available    bool
	Subject      string
	Organization string
	ValidFrom    string
	ValidUntil   string
	Fingerprint  string
	CertPath     string
	InstallURL   string
	PageURL      string
	HTTPSEnabled bool
}

// HandleCAPage renders the CA certificate trust page.
func (h *Handler) HandleCAPage(c echo.Context) error {
	data := CAPageData{
		BasePage:     h.basePage(),
		HTTPSEnabled: h.cfg.Routing.HTTPS.Enabled,
	}

	caCertPath := filepath.Join(h.cfg.DataPath("traefik"), "certs", "ca.crt")
	data.CertPath = caCertPath
	ipBase := h.caIPBaseURL()
	data.PageURL = ipBase + "/ca"
	data.InstallURL = ipBase

	cert, err := loadX509CertFromFile(caCertPath)
	if err == nil {
		data.Available = true
		data.Subject = cert.Subject.CommonName
		if len(cert.Subject.Organization) > 0 {
			data.Organization = cert.Subject.Organization[0]
		}
		data.ValidFrom = cert.NotBefore.Format("2006-01-02")
		data.ValidUntil = cert.NotAfter.Format("2006-01-02")
		fingerprint := sha256.Sum256(cert.Raw)
		data.Fingerprint = fmt.Sprintf("%X", fingerprint)
	}

	return c.Render(http.StatusOK, "ca.html", data)
}

// HandleCACert serves the CA certificate file for download.
func (h *Handler) HandleCACert(c echo.Context) error {
	caCertPath := filepath.Join(h.cfg.DataPath("traefik"), "certs", "ca.crt")

	data, err := os.ReadFile(caCertPath)
	if err != nil {
		return c.String(http.StatusNotFound, "CA certificate not available. Deploy traefik first.")
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=homelabctl-ca.crt")
	return c.Blob(http.StatusOK, "application/x-x509-ca-cert", data)
}

// HandleCAInstallScript serves a shell script that installs the CA certificate.
func (h *Handler) HandleCAInstallScript(c echo.Context) error {
	baseURL := h.caIPBaseURL()

	script := fmt.Sprintf(`#!/bin/bash
set -e

echo "Installing homelabctl CA certificate..."

CERT=$(curl -sS "%s/ca/cert")
if [ -z "$CERT" ]; then
    echo "Error: failed to download CA certificate."
    exit 1
fi

TMP=$(mktemp /tmp/homelabctl-ca.XXXXXX.crt)
echo "$CERT" > "$TMP"
trap "rm -f '$TMP'" EXIT

case "$(uname)" in
    Linux)
        # System trust store (for curl, wget, etc.)
        sudo cp "$TMP" /usr/local/share/ca-certificates/homelabctl-ca.crt
        sudo update-ca-certificates

        # Browser trust stores (Chrome/Firefox use NSS)
        # Resolve real user's home when running under sudo
        if [ -n "$SUDO_USER" ]; then
            USER_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
        else
            USER_HOME="$HOME"
        fi

        if ! command -v certutil >/dev/null 2>&1; then
            echo "Installing NSS tools for browser trust store support..."
            if command -v apt-get >/dev/null 2>&1; then
                apt-get install -y libnss3-tools >/dev/null 2>&1 || true
            elif command -v dnf >/dev/null 2>&1; then
                dnf install -y nss-tools >/dev/null 2>&1 || true
            elif command -v yum >/dev/null 2>&1; then
                yum install -y nss-tools >/dev/null 2>&1 || true
            elif command -v pacman >/dev/null 2>&1; then
                pacman -S --noconfirm nss >/dev/null 2>&1 || true
            elif command -v zypper >/dev/null 2>&1; then
                zypper install -y mozilla-nss-tools >/dev/null 2>&1 || true
            fi
        fi

        if command -v certutil >/dev/null 2>&1; then
            # Chrome / Chromium
            NSSDB="$USER_HOME/.pki/nssdb"
            if [ -d "$NSSDB" ]; then
                certutil -d sql:"$NSSDB" -D -n "homelabctl CA" 2>/dev/null || true
                certutil -d sql:"$NSSDB" -A -t "C,," -n "homelabctl CA" -i "$TMP"
                echo "Installed into Chrome trust store."
            fi
            # Firefox (all profiles)
            for profile in "$USER_HOME"/.mozilla/firefox/*/; do
                if [ -f "${profile}cert9.db" ]; then
                    certutil -d sql:"$profile" -D -n "homelabctl CA" 2>/dev/null || true
                    certutil -d sql:"$profile" -A -t "C,," -n "homelabctl CA" -i "$TMP"
                    echo "Installed into Firefox profile: $(basename "$profile")"
                fi
            done
        else
            echo ""
            echo "Note: install 'libnss3-tools' to also trust in Chrome/Firefox:"
            echo "  sudo apt install libnss3-tools && curl -sSL %s/ca/install.sh | sudo bash"
        fi
        echo "Restart your browser for changes to take effect."
        ;;
    Darwin)
        sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$TMP"
        ;;
    *)
        echo "Unsupported OS: $(uname)"
        echo "Download the certificate manually: %s/ca/cert"
        exit 1
        ;;
esac

echo "homelabctl CA certificate installed successfully."
`, baseURL, baseURL, baseURL)

	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(script))
}

// HandleCAQRCode serves a QR code PNG pointing to the CA trust page.
func (h *Handler) HandleCAQRCode(c echo.Context) error {
	url := h.caIPBaseURL() + "/ca"

	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to generate QR code")
	}

	return c.Blob(http.StatusOK, "image/png", png)
}

// caIPBaseURL returns the base HTTP URL using the server's IP address.
// IP is used instead of hostname because mDNS is unreliable on mobile devices.
func (h *Handler) caIPBaseURL() string {
	ip := netutil.DetectLocalIP()
	if ip == "" {
		ip = h.cfg.Hostname + "." + h.cfg.Network.Domain
	}
	return "http://" + ip + ":" + strconv.Itoa(h.cfg.Network.WebPort)
}

func loadX509CertFromFile(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

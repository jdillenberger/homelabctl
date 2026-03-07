package app

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestCertManagerEnsureCerts(t *testing.T) {
	tmpDir := t.TempDir()
	cm := NewCertManager(tmpDir)
	domain := "myhost.local"

	if err := cm.EnsureCerts([]string{domain}); err != nil {
		t.Fatalf("EnsureCerts() error: %v", err)
	}

	certsDir := filepath.Join(tmpDir, "certs")

	t.Run("CA files exist", func(t *testing.T) {
		for _, name := range []string{"ca.key", "ca.crt"} {
			path := filepath.Join(certsDir, name)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("expected %s to exist: %v", name, err)
			}
		}
	})

	t.Run("wildcard files exist", func(t *testing.T) {
		for _, name := range []string{"wildcard.key", "wildcard.crt"} {
			path := filepath.Join(certsDir, name)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("expected %s to exist: %v", name, err)
			}
		}
	})

	t.Run("dynamic/tls.yml exists", func(t *testing.T) {
		dynamicDir := filepath.Join(tmpDir, "dynamic")
		path := filepath.Join(dynamicDir, "tls.yml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected dynamic/tls.yml to exist: %v", err)
		}
		content := string(data)
		if !contains(content, "/certs/wildcard.crt") || !contains(content, "/certs/wildcard.key") {
			t.Error("tls.yml should reference /certs/wildcard.{crt,key}")
		}
		if !contains(content, "stores:") || !contains(content, "defaultCertificate:") {
			t.Error("tls.yml should configure default certificate store")
		}
	})

	t.Run("CA cert is a valid CA", func(t *testing.T) {
		cert := loadTestCert(t, filepath.Join(certsDir, "ca.crt"))
		if !cert.IsCA {
			t.Error("CA cert should have IsCA=true")
		}
		if cert.Subject.CommonName != "homelabctl Local CA" {
			t.Errorf("CA CN = %q, want 'homelabctl Local CA'", cert.Subject.CommonName)
		}
	})

	t.Run("wildcard cert has correct SANs", func(t *testing.T) {
		cert := loadTestCert(t, filepath.Join(certsDir, "wildcard.crt"))
		wantDNS := map[string]bool{domain: true}
		for _, dns := range cert.DNSNames {
			delete(wantDNS, dns)
		}
		if len(wantDNS) > 0 {
			t.Errorf("wildcard cert missing DNS names: %v (has %v)", wantDNS, cert.DNSNames)
		}
	})

	t.Run("wildcard cert signed by CA", func(t *testing.T) {
		caCert := loadTestCert(t, filepath.Join(certsDir, "ca.crt"))
		wildcardCert := loadTestCert(t, filepath.Join(certsDir, "wildcard.crt"))
		if err := wildcardCert.CheckSignatureFrom(caCert); err != nil {
			t.Errorf("wildcard cert not signed by CA: %v", err)
		}
	})

	t.Run("CACertPath returns correct path", func(t *testing.T) {
		expected := filepath.Join(certsDir, "ca.crt")
		if cm.CACertPath() != expected {
			t.Errorf("CACertPath() = %q, want %q", cm.CACertPath(), expected)
		}
	})
}

func TestCertManagerIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cm := NewCertManager(tmpDir)
	domain := "myhost.local"

	if err := cm.EnsureCerts([]string{domain}); err != nil {
		t.Fatalf("first EnsureCerts() error: %v", err)
	}

	// Read CA cert content
	caPath := filepath.Join(tmpDir, "certs", "ca.crt")
	caBefore, _ := os.ReadFile(caPath)

	// Run again — should not regenerate CA
	if err := cm.EnsureCerts([]string{domain}); err != nil {
		t.Fatalf("second EnsureCerts() error: %v", err)
	}

	caAfter, _ := os.ReadFile(caPath)
	if string(caBefore) != string(caAfter) {
		t.Error("CA cert should not be regenerated on second call")
	}
}

func loadTestCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("no PEM block in %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing certificate %s: %v", path, err)
	}
	return cert
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

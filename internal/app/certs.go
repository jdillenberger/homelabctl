package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CertManager generates and manages local CA and wildcard certificates.
type CertManager struct {
	certsDir   string
	dynamicDir string
}

// NewCertManager creates a new CertManager that stores certs in {dataDir}/certs
// and dynamic traefik config in {dataDir}/dynamic.
func NewCertManager(dataDir string) *CertManager {
	return &CertManager{
		certsDir:   filepath.Join(dataDir, "certs"),
		dynamicDir: filepath.Join(dataDir, "dynamic"),
	}
}

// CACertPath returns the path to the CA certificate.
func (cm *CertManager) CACertPath() string {
	return filepath.Join(cm.certsDir, "ca.crt")
}

// EnsureCerts generates the local CA (if missing) and a wildcard certificate
// for the given domain (if missing or expired).
func (cm *CertManager) EnsureCerts(domain string) error {
	if err := os.MkdirAll(cm.certsDir, 0o755); err != nil {
		return fmt.Errorf("creating certs directory: %w", err)
	}
	if err := os.MkdirAll(cm.dynamicDir, 0o755); err != nil {
		return fmt.Errorf("creating dynamic directory: %w", err)
	}

	caKeyPath := filepath.Join(cm.certsDir, "ca.key")
	caCrtPath := cm.CACertPath()

	// Generate CA if missing
	if !fileExists(caKeyPath) || !fileExists(caCrtPath) {
		if err := cm.generateCA(caKeyPath, caCrtPath); err != nil {
			return fmt.Errorf("generating CA: %w", err)
		}
	}

	wildcardKeyPath := filepath.Join(cm.certsDir, "wildcard.key")
	wildcardCrtPath := filepath.Join(cm.certsDir, "wildcard.crt")

	// Generate wildcard cert if missing or expired
	needsRegen := !fileExists(wildcardKeyPath) || !fileExists(wildcardCrtPath)
	if !needsRegen {
		needsRegen = cm.isCertExpired(wildcardCrtPath)
	}

	if needsRegen {
		if err := cm.generateWildcard(domain, caKeyPath, caCrtPath, wildcardKeyPath, wildcardCrtPath); err != nil {
			return fmt.Errorf("generating wildcard cert: %w", err)
		}
	}

	// Write dynamic/tls.yml for Traefik file provider
	tlsYml := `tls:
  certificates:
    - certFile: /certs/wildcard.crt
      keyFile: /certs/wildcard.key
`
	if err := os.WriteFile(filepath.Join(cm.dynamicDir, "tls.yml"), []byte(tlsYml), 0o600); err != nil {
		return fmt.Errorf("writing tls.yml: %w", err)
	}

	return nil
}

func (cm *CertManager) generateCA(keyPath, crtPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "homelabctl Local CA",
			Organization: []string{"homelabctl"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}

	if err := writePEM(keyPath, "EC PRIVATE KEY", key); err != nil {
		return err
	}
	return writeCertPEM(crtPath, certDER)
}

func (cm *CertManager) generateWildcard(domain, caKeyPath, caCrtPath, keyPath, crtPath string) error {
	caKey, err := loadECKey(caKeyPath)
	if err != nil {
		return fmt.Errorf("loading CA key: %w", err)
	}
	caCert, err := loadCert(caCrtPath)
	if err != nil {
		return fmt.Errorf("loading CA cert: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "*." + domain,
			Organization: []string{"homelabctl"},
		},
		DNSNames:  []string{domain, "*." + domain},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}

	if err := writePEM(keyPath, "EC PRIVATE KEY", key); err != nil {
		return err
	}
	return writeCertPEM(crtPath, certDER)
}

func (cm *CertManager) isCertExpired(certPath string) bool {
	cert, err := loadCert(certPath)
	if err != nil {
		return true
	}
	// Renew 30 days before expiry
	return time.Now().After(cert.NotAfter.Add(-30 * 24 * time.Hour))
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func writePEM(path, typ string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: typ, Bytes: der})
}

func writeCertPEM(path string, der []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func loadECKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func loadCert(path string) (*x509.Certificate, error) {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

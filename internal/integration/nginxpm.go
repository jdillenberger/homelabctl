package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jdillenberger/homelabctl/internal/config"
)

// NPMClient interacts with the Nginx Proxy Manager API.
type NPMClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewNPMClient creates a new NPM API client from config.
func NewNPMClient(cfg config.NPMConfig) *NPMClient {
	return &NPMClient{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// npmTokenRequest is the login request payload.
type npmTokenRequest struct {
	Identity string `json:"identity"`
	Secret   string `json:"secret"`
}

// npmTokenResponse is the login response payload.
type npmTokenResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

// Authenticate obtains a JWT token from the NPM API.
func (nc *NPMClient) Authenticate(email, password string) error {
	tokenURL := fmt.Sprintf("%s/api/tokens", nc.baseURL)

	payload := npmTokenRequest{
		Identity: email,
		Secret:   password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling login payload: %w", err)
	}

	resp, err := nc.httpClient.Post(tokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("npm auth request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading npm auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("npm auth failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp npmTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("parsing npm token response: %w", err)
	}

	if tokenResp.Token == "" {
		return fmt.Errorf("npm auth returned empty token")
	}

	nc.token = tokenResp.Token
	return nil
}

// NPMProxyHost represents a proxy host entry in Nginx Proxy Manager.
type NPMProxyHost struct {
	ID                int      `json:"id"`
	DomainNames       []string `json:"domain_names"`
	ForwardHost       string   `json:"forward_host"`
	ForwardPort       int      `json:"forward_port"`
	ForwardScheme     string   `json:"forward_scheme"`
	CertificateID     interface{} `json:"certificate_id"`
	SSLForced         bool     `json:"ssl_forced"`
	BlockExploits     bool     `json:"block_exploits"`
	AllowWebsocketUpgrade bool `json:"allow_websocket_upgrade"`
}

// npmCreateProxyHostRequest is the payload for creating a proxy host.
type npmCreateProxyHostRequest struct {
	DomainNames           []string `json:"domain_names"`
	ForwardHost           string   `json:"forward_host"`
	ForwardPort           int      `json:"forward_port"`
	ForwardScheme         string   `json:"forward_scheme"`
	CertificateID         string   `json:"certificate_id"`
	SSLForced             bool     `json:"ssl_forced"`
	BlockExploits         bool     `json:"block_exploits"`
	AllowWebsocketUpgrade bool     `json:"allow_websocket_upgrade"`
	Meta                  struct {
		LetsencryptAgree bool   `json:"letsencrypt_agree"`
		DNSChallenge     bool   `json:"dns_challenge"`
	} `json:"meta"`
}

// CreateProxyHost creates a new proxy host entry in NPM.
func (nc *NPMClient) CreateProxyHost(domain string, forwardHost string, forwardPort int, ssl bool) error {
	if nc.token == "" {
		return fmt.Errorf("not authenticated — call Authenticate() first")
	}

	hostsURL := fmt.Sprintf("%s/api/nginx/proxy-hosts", nc.baseURL)

	scheme := "http"
	if ssl {
		scheme = "https"
	}

	payload := npmCreateProxyHostRequest{
		DomainNames:           []string{domain},
		ForwardHost:           forwardHost,
		ForwardPort:           forwardPort,
		ForwardScheme:         scheme,
		CertificateID:         "new",
		SSLForced:             ssl,
		BlockExploits:         true,
		AllowWebsocketUpgrade: true,
	}
	payload.Meta.LetsencryptAgree = ssl
	payload.Meta.DNSChallenge = false

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling proxy host payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, hostsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+nc.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("npm create proxy host request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("npm create proxy host failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteProxyHost deletes a proxy host by its ID.
func (nc *NPMClient) DeleteProxyHost(id int) error {
	if nc.token == "" {
		return fmt.Errorf("not authenticated — call Authenticate() first")
	}

	hostURL := fmt.Sprintf("%s/api/nginx/proxy-hosts/%d", nc.baseURL, id)

	req, err := http.NewRequest(http.MethodDelete, hostURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+nc.token)

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("npm delete proxy host request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("npm delete proxy host failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListProxyHosts returns all proxy host entries from NPM.
func (nc *NPMClient) ListProxyHosts() ([]NPMProxyHost, error) {
	if nc.token == "" {
		return nil, fmt.Errorf("not authenticated — call Authenticate() first")
	}

	hostsURL := fmt.Sprintf("%s/api/nginx/proxy-hosts", nc.baseURL)

	req, err := http.NewRequest(http.MethodGet, hostsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+nc.token)

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm list proxy hosts request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading npm list response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm list proxy hosts failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var hosts []NPMProxyHost
	if err := json.Unmarshal(body, &hosts); err != nil {
		return nil, fmt.Errorf("parsing npm proxy hosts list: %w", err)
	}

	return hosts, nil
}

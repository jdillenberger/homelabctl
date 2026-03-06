package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jdillenberger/homelabctl/internal/config"
)

// KeycloakClient interacts with the Keycloak Admin REST API.
type KeycloakClient struct {
	baseURL     string
	realm       string
	adminUser   string
	adminPass   string
	accessToken string
	httpClient  *http.Client
}

// NewKeycloakClient creates a new Keycloak API client from config.
func NewKeycloakClient(cfg config.KeycloakConfig, adminUser, adminPass string) *KeycloakClient {
	return &KeycloakClient{
		baseURL:   strings.TrimRight(cfg.URL, "/"),
		realm:     cfg.Realm,
		adminUser: adminUser,
		adminPass: adminPass,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// keycloakTokenResponse represents the OAuth2 token endpoint response.
type keycloakTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// keycloakErrorResponse represents a Keycloak error response.
type keycloakErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"errorMessage"`
}

// Authenticate obtains an admin access token from the master realm.
func (kc *KeycloakClient) Authenticate() error {
	tokenURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", kc.baseURL)

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", kc.adminUser)
	form.Set("password", kc.adminPass)

	resp, err := kc.httpClient.PostForm(tokenURL, form)
	if err != nil {
		return fmt.Errorf("keycloak auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading keycloak auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp keycloakErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("keycloak auth failed (HTTP %d): %s — %s", resp.StatusCode, errResp.Error, errResp.ErrorDescription)
		}
		return fmt.Errorf("keycloak auth failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp keycloakTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parsing keycloak token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("keycloak auth returned empty access token")
	}

	kc.accessToken = tokenResp.AccessToken
	return nil
}

// keycloakClientRepresentation is the Keycloak client JSON payload.
type keycloakClientRepresentation struct {
	ClientID                  string   `json:"clientId"`
	Name                      string   `json:"name"`
	Enabled                   bool     `json:"enabled"`
	Protocol                  string   `json:"protocol"`
	PublicClient              bool     `json:"publicClient"`
	RedirectURIs              []string `json:"redirectUris"`
	StandardFlowEnabled       bool     `json:"standardFlowEnabled"`
	DirectAccessGrantsEnabled bool     `json:"directAccessGrantsEnabled"`
}

// CreateOIDCClient creates a new OIDC client in the configured realm.
func (kc *KeycloakClient) CreateOIDCClient(clientID, clientName, redirectURI string) error {
	if kc.accessToken == "" {
		return fmt.Errorf("not authenticated — call Authenticate() first")
	}

	clientsURL := fmt.Sprintf("%s/admin/realms/%s/clients", kc.baseURL, url.PathEscape(kc.realm))

	payload := keycloakClientRepresentation{
		ClientID:                  clientID,
		Name:                      clientName,
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              false,
		RedirectURIs:              []string{redirectURI},
		StandardFlowEnabled:       true,
		DirectAccessGrantsEnabled: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling client payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, clientsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+kc.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := kc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak create client request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak create client failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteClient removes a client by its internal UUID. Use getClientUUID to resolve
// a clientID to its UUID first.
func (kc *KeycloakClient) DeleteClient(clientID string) error {
	if kc.accessToken == "" {
		return fmt.Errorf("not authenticated — call Authenticate() first")
	}

	uuid, err := kc.getClientUUID(clientID)
	if err != nil {
		return fmt.Errorf("resolving client UUID: %w", err)
	}

	clientURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s", kc.baseURL, url.PathEscape(kc.realm), uuid)

	req, err := http.NewRequest(http.MethodDelete, clientURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+kc.accessToken)

	resp, err := kc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak delete client request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak delete client failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// keycloakClientSecretResponse represents the client secret endpoint response.
type keycloakClientSecretResponse struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// GetClientSecret retrieves the client secret for a given clientID.
func (kc *KeycloakClient) GetClientSecret(clientID string) (string, error) {
	if kc.accessToken == "" {
		return "", fmt.Errorf("not authenticated — call Authenticate() first")
	}

	uuid, err := kc.getClientUUID(clientID)
	if err != nil {
		return "", fmt.Errorf("resolving client UUID: %w", err)
	}

	secretURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s/client-secret", kc.baseURL, url.PathEscape(kc.realm), uuid)

	req, err := http.NewRequest(http.MethodGet, secretURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+kc.accessToken)

	resp, err := kc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak get secret request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading keycloak secret response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak get secret failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var secretResp keycloakClientSecretResponse
	if err := json.Unmarshal(body, &secretResp); err != nil {
		return "", fmt.Errorf("parsing keycloak secret response: %w", err)
	}

	return secretResp.Value, nil
}

// keycloakClientListEntry is a minimal client object returned by the list endpoint.
type keycloakClientListEntry struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
}

// getClientUUID resolves a clientID (e.g. "my-app") to its internal Keycloak UUID.
func (kc *KeycloakClient) getClientUUID(clientID string) (string, error) {
	listURL := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", kc.baseURL, url.PathEscape(kc.realm), url.QueryEscape(clientID))

	req, err := http.NewRequest(http.MethodGet, listURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+kc.accessToken)

	resp, err := kc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak list clients request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading keycloak list response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak list clients failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var clients []keycloakClientListEntry
	if err := json.Unmarshal(body, &clients); err != nil {
		return "", fmt.Errorf("parsing keycloak clients list: %w", err)
	}

	for _, c := range clients {
		if c.ClientID == clientID {
			return c.ID, nil
		}
	}

	return "", fmt.Errorf("keycloak client %q not found in realm %q", clientID, kc.realm)
}

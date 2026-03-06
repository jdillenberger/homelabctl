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

// BeszelClient interacts with the Beszel hub API for host registration.
type BeszelClient struct {
	hubURL     string
	httpClient *http.Client
}

// NewBeszelClient creates a new Beszel hub API client from config.
func NewBeszelClient(cfg config.BeszelConfig) *BeszelClient {
	return &BeszelClient{
		hubURL: strings.TrimRight(cfg.HubURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// beszelRegisterRequest is the JSON payload for host registration.
type beszelRegisterRequest struct {
	Hostname string `json:"hostname"`
	Address  string `json:"address"`
}

// RegisterHost registers a host with the Beszel hub.
func (bc *BeszelClient) RegisterHost(hostname, address string) error {
	registerURL := fmt.Sprintf("%s/api/hosts", bc.hubURL)

	payload := beszelRegisterRequest{
		Hostname: hostname,
		Address:  address,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling register payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, registerURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("beszel register host request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("beszel register host failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// UnregisterHost removes a host from the Beszel hub.
func (bc *BeszelClient) UnregisterHost(hostname string) error {
	unregisterURL := fmt.Sprintf("%s/api/hosts/%s", bc.hubURL, hostname)

	req, err := http.NewRequest(http.MethodDelete, unregisterURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("beszel unregister host request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("beszel unregister host failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

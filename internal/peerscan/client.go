package peerscan

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Peer represents a single peer in the fleet.
type Peer struct {
	Hostname string            `json:"hostname"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Version  string            `json:"version"`
	Role     string            `json:"role"`
	Tags     map[string]string `json:"tags,omitempty"`
	Online   bool              `json:"online"`
}

// PeersResponse is the response from GET /api/peers.
type PeersResponse struct {
	Fleet struct {
		Name string `json:"name"`
	} `json:"fleet"`
	Self  Peer   `json:"self"`
	Peers []Peer `json:"peers"`
}

// HealthResponse is the response from GET /api/health.
type HealthResponse struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Uptime   int64  `json:"uptime_seconds"`
}

// Client talks to the local peer-scanner REST API.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient creates a new peer-scanner API client.
func NewClient(url, secret string) *Client {
	return &Client{
		baseURL: url,
		secret:  secret,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Peers returns all known peers from the peer-scanner daemon.
func (c *Client) Peers() (*PeersResponse, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/peers", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying peer-scanner: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("peer-scanner returned %d: %s", resp.StatusCode, string(body))
	}

	var result PeersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// Health returns the health status of the peer-scanner daemon.
func (c *Client) Health() (*HealthResponse, error) {
	resp, err := c.http.Get(c.baseURL + "/api/health")
	if err != nil {
		return nil, fmt.Errorf("querying peer-scanner health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("peer-scanner returned %d: %s", resp.StatusCode, string(body))
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

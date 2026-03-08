package alertclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Event represents an event pushed to labalert's /api/events endpoint.
type Event struct {
	Type     string `json:"type"`
	App      string `json:"app,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Client is a thin HTTP client for labalert's API.
type Client struct {
	baseURL string
	client  *http.Client
}

// New creates a new labalert API client.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PushEvent sends an event to labalert for rule evaluation and notification.
func (c *Client) PushEvent(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting event to labalert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("labalert returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Rules fetches alert rules from labalert.
func (c *Client) Rules(ctx context.Context) (json.RawMessage, error) {
	return c.getJSON(ctx, "/api/rules")
}

// History fetches alert history from labalert.
func (c *Client) History(ctx context.Context, limit int) (json.RawMessage, error) {
	path := "/api/history"
	if limit > 0 {
		path = fmt.Sprintf("/api/history?limit=%d", limit)
	}
	return c.getJSON(ctx, path)
}

func (c *Client) getJSON(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("labalert returned HTTP %d for %s", resp.StatusCode, path)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return raw, nil
}

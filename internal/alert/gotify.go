package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GotifyNotifier sends alerts via a Gotify server.
type GotifyNotifier struct {
	url    string // e.g. "https://gotify.example.com"
	token  string
	client *http.Client
}

// NewGotifyNotifier creates a new GotifyNotifier.
func NewGotifyNotifier(url, token string) *GotifyNotifier {
	return &GotifyNotifier{
		url:    url,
		token:  token,
		client: &http.Client{},
	}
}

func (g *GotifyNotifier) Name() string { return "gotify" }

func (g *GotifyNotifier) Send(ctx context.Context, a Alert) error {
	priority := 5
	switch a.Severity {
	case SeverityCritical:
		priority = 8
	case SeverityWarning:
		priority = 5
	case SeverityInfo:
		priority = 2
	}

	msg := map[string]interface{}{
		"title":    fmt.Sprintf("[%s] %s", a.Severity, a.Type),
		"message":  fmt.Sprintf("%s\n%s", a.Message, a.Detail),
		"priority": priority,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling gotify message: %w", err)
	}

	endpoint := fmt.Sprintf("%s/message?token=%s", g.url, g.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating gotify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending gotify notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gotify returned HTTP %d", resp.StatusCode)
	}
	return nil
}

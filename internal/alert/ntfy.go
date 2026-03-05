package alert

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// NtfyNotifier sends alerts via ntfy.sh or a self-hosted ntfy server.
type NtfyNotifier struct {
	url    string // e.g. "https://ntfy.sh/my-homelab"
	token  string
	client *http.Client
}

// NewNtfyNotifier creates a new NtfyNotifier.
func NewNtfyNotifier(url, token string) *NtfyNotifier {
	return &NtfyNotifier{
		url:    url,
		token:  token,
		client: &http.Client{},
	}
}

func (n *NtfyNotifier) Name() string { return "ntfy" }

func (n *NtfyNotifier) Send(ctx context.Context, a Alert) error {
	body := fmt.Sprintf("%s\n%s", a.Message, a.Detail)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("creating ntfy request: %w", err)
	}

	req.Header.Set("Title", fmt.Sprintf("[%s] %s", a.Severity, a.Type))
	switch a.Severity {
	case SeverityCritical:
		req.Header.Set("Priority", "urgent")
		req.Header.Set("Tags", "rotating_light")
	case SeverityWarning:
		req.Header.Set("Priority", "high")
		req.Header.Set("Tags", "warning")
	default:
		req.Header.Set("Priority", "default")
		req.Header.Set("Tags", "information_source")
	}

	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending ntfy notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
	}
	return nil
}

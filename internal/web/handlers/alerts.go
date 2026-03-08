package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/alertclient"
)

// AlertsPageData holds data for the alerts template.
type AlertsPageData struct {
	BasePage
	Rules   json.RawMessage
	History json.RawMessage
}

// HandleAlertsPage renders the alerts management page.
func (h *Handler) HandleAlertsPage(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	client := alertclient.New(h.cfg.Labalert.URL)

	rules, err := client.Rules(ctx)
	if err != nil {
		rules = json.RawMessage("[]")
	}

	history, err := client.History(ctx, 50)
	if err != nil {
		history = json.RawMessage("[]")
	}

	return c.Render(http.StatusOK, "alerts.html", AlertsPageData{
		BasePage: h.basePage(),
		Rules:    rules,
		History:  history,
	})
}

// AlertsPartial renders a compact list of recent alerts for the sidebar.
func (h *Handler) AlertsPartial(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	client := alertclient.New(h.cfg.Labalert.URL)
	history, err := client.History(ctx, 5)
	if err != nil {
		history = json.RawMessage("[]")
	}
	return c.Render(http.StatusOK, "alerts_partial.html", history)
}

// APIAlertRules returns alert rules as JSON (proxied from labalert).
func (h *Handler) APIAlertRules(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	client := alertclient.New(h.cfg.Labalert.URL)
	rules, err := client.Rules(ctx)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	return c.JSONBlob(http.StatusOK, rules)
}

// APIAlertHistory returns alert history as JSON (proxied from labalert).
func (h *Handler) APIAlertHistory(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	client := alertclient.New(h.cfg.Labalert.URL)
	history, err := client.History(ctx, 0)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	return c.JSONBlob(http.StatusOK, history)
}

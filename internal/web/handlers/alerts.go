package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jdillenberger/homelabctl/internal/alert"
)

// AlertsPageData holds data for the alerts template.
type AlertsPageData struct {
	BasePage
	Rules   []alert.Rule
	History []alert.Alert
}

// HandleAlertsPage renders the alerts management page.
func (h *Handler) HandleAlertsPage(c echo.Context) error {
	store := alert.NewStore(h.cfg.DataDir)

	rules, err := store.LoadRules()
	if err != nil {
		rules = nil
	}

	history, err := store.LoadHistory()
	if err != nil {
		history = nil
	}

	// Show last 50 entries
	if len(history) > 50 {
		history = history[len(history)-50:]
	}

	return c.Render(http.StatusOK, "alerts.html", AlertsPageData{
		BasePage: h.basePage(),
		Rules:    rules,
		History:  history,
	})
}

// APIAlertRules returns alert rules as JSON.
func (h *Handler) APIAlertRules(c echo.Context) error {
	store := alert.NewStore(h.cfg.DataDir)
	rules, err := store.LoadRules()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if rules == nil {
		rules = []alert.Rule{}
	}
	return c.JSON(http.StatusOK, rules)
}

// APIAlertHistory returns alert history as JSON.
func (h *Handler) APIAlertHistory(c echo.Context) error {
	store := alert.NewStore(h.cfg.DataDir)
	history, err := store.LoadHistory()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if history == nil {
		history = []alert.Alert{}
	}
	return c.JSON(http.StatusOK, history)
}

package alert

import (
	"context"
	"fmt"
	"net/smtp"
)

// EmailNotifier sends alerts via SMTP.
type EmailNotifier struct {
	host     string
	port     int
	from     string
	to       string
	username string
	password string
}

// NewEmailNotifier creates a new EmailNotifier.
func NewEmailNotifier(host string, port int, from, to, username, password string) *EmailNotifier {
	return &EmailNotifier{
		host:     host,
		port:     port,
		from:     from,
		to:       to,
		username: username,
		password: password,
	}
}

func (e *EmailNotifier) Name() string { return "email" }

func (e *EmailNotifier) Send(_ context.Context, a Alert) error {
	subject := fmt.Sprintf("[homelabctl] [%s] %s", a.Severity, a.Type)
	body := fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nTo: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\n\n%s\n\nTimestamp: %s\n",
		subject, e.from, e.to, a.Message, a.Detail, a.Timestamp.Format("2006-01-02 15:04:05"))

	addr := fmt.Sprintf("%s:%d", e.host, e.port)
	var auth smtp.Auth
	if e.username != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}

	if err := smtp.SendMail(addr, auth, e.from, []string{e.to}, []byte(body)); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	return nil
}

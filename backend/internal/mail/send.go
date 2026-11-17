package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Outgoing is one email we send. Laamo sends only to people who asked for it
// (their own sign-up or their own account), never to third parties.
type Outgoing struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// IdempotencyKey: the same key twice sends once (retries are safe).
	IdempotencyKey string
}

// Sender sends an email.
type Sender interface {
	Send(ctx context.Context, m Outgoing) error
}

// Resend sends through the Resend API (outgoing mail provider, see the
// product plan: notify.laamo.app, only to the person themselves).
type Resend struct {
	APIKey string
	From   string // e.g. "Laamo <hello@notify.laamo.app>"
	URL    string // default https://api.resend.com/emails
	Client *http.Client
}

func (r *Resend) Send(ctx context.Context, m Outgoing) error {
	body, err := json.Marshal(map[string]any{
		"from": r.From, "to": []string{m.To}, "subject": m.Subject, "text": m.Text, "html": m.HTML,
	})
	if err != nil {
		return err
	}
	url := r.URL
	if url == "" {
		url = "https://api.resend.com/emails"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if m.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", m.IdempotencyKey)
	}
	cl := r.Client
	if cl == nil {
		cl = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := cl.Do(req)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("resend: %d %s", res.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}

// LogSender writes emails to the log instead of sending them (local dev
// without a Resend key): copy the link from the api's output.
type LogSender struct{ Log *slog.Logger }

func (l LogSender) Send(_ context.Context, m Outgoing) error {
	l.Log.Info("email (not sent: no RESEND_API_KEY)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

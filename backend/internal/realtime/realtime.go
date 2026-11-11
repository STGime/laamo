// Package realtime listens to Eurobase realtime (table change events) with
// the secret key, from the server. Browsers don't connect to Eurobase
// directly: the API forwards each member only their workspace's events.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Event is one change. Type is INSERT, UPDATE or DELETE; Reconnected is a
// synthetic event sent after the connection came back, because changes may
// have been missed meanwhile (listeners should reload).
type Event struct {
	Channel string         `json:"channel"`
	Type    string         `json:"type"`
	Record  map[string]any `json:"record"`
	Error   string         `json:"error"`
}

const Reconnected = "RECONNECTED"

// Table returns the table name of a "db:<table>" channel.
func (e Event) Table() string { return strings.TrimPrefix(e.Channel, "db:") }

// String returns a text field of the record ("" if missing).
func (e Event) String(field string) string {
	s, _ := e.Record[field].(string)
	return s
}

// Subscribe listens to the tables' channels until ctx ends, reconnecting
// with backoff. Events can be lost while disconnected; a Reconnected event
// follows every reconnect. The channel is buffered; when a consumer falls
// behind, events are dropped rather than blocking the connection.
func Subscribe(ctx context.Context, baseURL, secretKey string, tables []string, log *slog.Logger) <-chan Event {
	if log == nil {
		log = slog.Default()
	}
	ch := make(chan Event, 256)
	go func() {
		defer close(ch)
		wait := time.Second
		for connected := false; ctx.Err() == nil; connected = true {
			start := time.Now()
			err := listen(ctx, baseURL, secretKey, tables, ch, connected)
			if ctx.Err() != nil {
				return
			}
			if time.Since(start) > time.Minute {
				wait = time.Second // it was up for a while: reconnect quickly
			}
			log.Warn("realtime disconnected", "err", err, "retry_in", wait)
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			wait = min(wait*2, 30*time.Second)
		}
	}()
	return ch
}

// URL returns the realtime WebSocket URL for a project base URL.
func URL(baseURL, key string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("realtime: unsupported URL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/realtime"
	u.RawQuery = url.Values{"token": {key}}.Encode()
	return u.String(), nil
}

func send(ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	default: // consumer is behind; drop
	}
}

func listen(ctx context.Context, baseURL, key string, tables []string, ch chan<- Event, reconnect bool) error {
	target, err := URL(baseURL, key)
	if err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// The secret key is the token query parameter (the only way Eurobase
	// realtime accepts it); Origin is the project URL, which its allowlist
	// accepts.
	conn, resp, err := websocket.Dial(dctx, target, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {strings.TrimRight(baseURL, "/")}},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("realtime: dial: %w", Redact(err, key))
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(1 << 20)

	for _, t := range tables {
		sub, _ := json.Marshal(map[string]string{"action": "subscribe", "channel": "db:" + t})
		if err := conn.Write(ctx, websocket.MessageText, sub); err != nil {
			return fmt.Errorf("realtime: subscribe: %w", err)
		}
	}
	if reconnect {
		send(ch, Event{Type: Reconnected})
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("realtime: read: %w", err)
		}
		var ev Event
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		switch {
		case ev.Error != "":
			return fmt.Errorf("realtime: server error: %s", ev.Error)
		case strings.HasPrefix(ev.Channel, "db:") && ev.Type != "":
			send(ch, ev)
		}
	}
}

// Redact removes the key, plain or URL-encoded, from an error (dial errors
// include the URL).
func Redact(err error, key string) error {
	if err == nil || key == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), url.QueryEscape(key), "REDACTED")
	return errors.New(strings.ReplaceAll(msg, key, "REDACTED"))
}

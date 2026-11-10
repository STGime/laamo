package eurobase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Wakeups subscribes to Eurobase realtime and sends on the returned channel
// whenever a job is inserted through REST (Enqueue). It's only a hint for
// idle workers to claim now instead of at the next poll: cron-inserted jobs
// fire no event, and events can be missed while reconnecting.
//
// It reconnects with backoff until ctx ends. The secret key is sent as the
// token query parameter (the only way Eurobase realtime accepts it), and
// Origin is the project URL, which Eurobase's allowlist accepts.
func Wakeups(ctx context.Context, baseURL, secretKey string, log *slog.Logger) <-chan struct{} {
	if log == nil {
		log = slog.Default()
	}
	ch := make(chan struct{}, 1)
	go func() {
		wait := time.Second
		for ctx.Err() == nil {
			start := time.Now()
			err := listen(ctx, baseURL, secretKey, ch)
			if ctx.Err() != nil {
				return
			}
			if time.Since(start) > time.Minute {
				wait = time.Second // it was up for a while: reconnect quickly
			}
			log.Warn("realtime wake-up disconnected; polling continues", "err", err, "retry_in", wait)
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

func realtimeURL(baseURL, key string) (string, error) {
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

type event struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Error   string `json:"error"`
}

func listen(ctx context.Context, baseURL, key string, ch chan<- struct{}) error {
	target, err := realtimeURL(baseURL, key)
	if err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(dctx, target, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {strings.TrimRight(baseURL, "/")}},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("realtime: dial: %w", redact(err, key))
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(1 << 20)

	sub, _ := json.Marshal(map[string]string{"action": "subscribe", "channel": "db:jobs"})
	if err := conn.Write(ctx, websocket.MessageText, sub); err != nil {
		return fmt.Errorf("realtime: subscribe: %w", err)
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("realtime: read: %w", err)
		}
		var ev event
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		switch {
		case ev.Error != "":
			return fmt.Errorf("realtime: server error: %s", ev.Error)
		case ev.Channel == "db:jobs" && ev.Type == "INSERT":
			select {
			case ch <- struct{}{}:
			default: // a wake-up is already pending
			}
		}
	}
}

// redact removes the key from dial errors (the URL carries it).
func redact(err error, key string) error {
	if key == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), url.QueryEscape(key), "REDACTED")
	return fmt.Errorf("%s", strings.ReplaceAll(msg, key, "REDACTED"))
}

// Package chat posts messages to the user's chat places: Discord and Slack
// incoming webhooks (MVP); Telegram (#51) and Teams (#66) come later.
//
// Webhook URLs are secrets (anyone with one can post). They are stored
// encrypted and only decrypted in the worker right before sending.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrBadWebhook: not a Discord or Slack webhook URL.
	ErrBadWebhook = errors.New("chat: not a valid webhook URL")
	// ErrGone: the webhook was deleted or revoked; retrying won't help.
	ErrGone = errors.New("chat: the webhook doesn't work anymore")
	// ErrRejected: the chat answered with an error, so the message was not
	// delivered (safe to send again later).
	ErrRejected = errors.New("chat: message rejected")
	// ErrUnsupported: the place type can't be posted to yet.
	ErrUnsupported = errors.New("chat: this chat type isn't supported yet")
)

// Message is what to post. Text uses simple markdown (**bold**, links).
type Message struct {
	Text string
}

// Sender posts to webhooks.
type Sender struct {
	HTTP *http.Client
	// AllowAnyHost skips the webhook host check (tests with a local server).
	AllowAnyHost bool
}

const discordMax = 2000

// CheckWebhook validates a webhook URL for a place type.
func CheckWebhook(placeType, raw string) error {
	if placeType != "discord" && placeType != "slack" {
		return fmt.Errorf("%w: %s", ErrUnsupported, placeType)
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ErrBadWebhook
	}
	switch placeType {
	case "discord":
		if (u.Host == "discord.com" || u.Host == "discordapp.com" || u.Host == "canary.discord.com") && strings.HasPrefix(u.Path, "/api/webhooks/") {
			return nil
		}
	case "slack":
		if u.Host == "hooks.slack.com" && strings.HasPrefix(u.Path, "/services/") {
			return nil
		}
	}
	return ErrBadWebhook
}

// Send posts msg to a webhook of the given place type.
func (s *Sender) Send(ctx context.Context, placeType, webhook string, msg Message) error {
	if !s.AllowAnyHost {
		if err := CheckWebhook(placeType, webhook); err != nil {
			return err
		}
	}
	var body any
	switch placeType {
	case "discord":
		text := msg.Text
		if r := []rune(text); len(r) > discordMax {
			text = string(r[:discordMax-1]) + "…"
		}
		// No pings from content (@everyone in a page title stays text).
		body = map[string]any{"content": text, "allowed_mentions": map[string]any{"parse": []string{}}}
	case "slack":
		body = map[string]any{"text": slackText(msg.Text), "unfurl_links": false}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, placeType)
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		return ErrBadWebhook
	}
	req.Header.Set("Content-Type", "application/json")
	hc := s.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := hc.Do(req) //nolint:gosec // G704: host checked by CheckWebhook
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden,
		resp.StatusCode == http.StatusGone:
		return fmt.Errorf("%w (HTTP %d)", ErrGone, resp.StatusCode)
	default:
		return fmt.Errorf("%w: HTTP %d: %.200s", ErrRejected, resp.StatusCode, data)
	}
}

// slackText turns **bold** into Slack's *bold* and [text](url) into <url|text>.
func slackText(s string) string {
	s = strings.ReplaceAll(s, "**", "*")
	var b strings.Builder
	for {
		i := strings.Index(s, "[")
		j := strings.Index(s, "](")
		k := -1
		if j > i && i >= 0 {
			k = strings.Index(s[j:], ")")
		}
		if i < 0 || j < i || k < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString("<" + s[j+2:j+k] + "|" + s[i+1:j] + ">")
		s = s[j+k+1:]
	}
}

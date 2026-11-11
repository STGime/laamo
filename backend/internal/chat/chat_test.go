package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckWebhook(t *testing.T) {
	ok := map[string]string{
		"discord": "https://discord.com/api/webhooks/123/abcDEF",
		"slack":   "https://hooks.slack.com/services/T000/B000/XXXX",
	}
	for typ, u := range ok {
		if err := CheckWebhook(typ, u); err != nil {
			t.Errorf("%s: %v", typ, err)
		}
	}
	bad := [][2]string{
		{"discord", "http://discord.com/api/webhooks/1/x"},
		{"discord", "https://evil.com/api/webhooks/1/x"},
		{"discord", "https://discord.com.evil.com/api/webhooks/1/x"},
		{"discord", "https://user:pw@discord.com/api/webhooks/1/x"},
		{"slack", "https://hooks.slack.com/other"},
		{"slack", "https://discord.com/api/webhooks/1/x"},
		{"discord", "not a url"},
	}
	for _, c := range bad {
		if err := CheckWebhook(c[0], c[1]); !errors.Is(err, ErrBadWebhook) {
			t.Errorf("%s %s: %v", c[0], c[1], err)
		}
	}
	if err := CheckWebhook("telegram", "x"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("telegram: %v", err)
	}
}

func TestSend(t *testing.T) {
	var got map[string]any
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	s := &Sender{AllowAnyHost: true}
	ctx := context.Background()

	long := strings.Repeat("x", 2500) + " @everyone"
	if err := s.Send(ctx, "discord", srv.URL, Message{Text: long}); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(got["content"].(string))); n != 2000 {
		t.Fatalf("discord text cut to 2000, got %d", n)
	}
	if am, _ := got["allowed_mentions"].(map[string]any); am == nil || len(am["parse"].([]any)) != 0 {
		t.Fatalf("mentions must be off: %v", got["allowed_mentions"])
	}

	if err := s.Send(ctx, "slack", srv.URL, Message{Text: "**Morning Briefing**\n• [Flour prices](https://x.de/a)"}); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "*Morning Briefing*\n• <https://x.de/a|Flour prices>" {
		t.Fatalf("slack text: %q", got["text"])
	}

	status = http.StatusNotFound
	if err := s.Send(ctx, "discord", srv.URL, Message{Text: "x"}); !errors.Is(err, ErrGone) {
		t.Fatalf("deleted webhook: %v", err)
	}
	status = http.StatusTooManyRequests
	if err := s.Send(ctx, "discord", srv.URL, Message{Text: "x"}); !errors.Is(err, ErrRejected) || errors.Is(err, ErrGone) {
		t.Fatalf("429 is a rejection that can be retried: %v", err)
	}
	if err := (&Sender{}).Send(ctx, "discord", srv.URL, Message{Text: "x"}); !errors.Is(err, ErrBadWebhook) {
		t.Fatalf("local URL without AllowAnyHost: %v", err)
	}
}

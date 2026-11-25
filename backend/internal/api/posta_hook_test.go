package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/store"
)

const hookSecret = "whsec_test"

// A Posta account with a registered webhook and one post Laamo made.
func hookSetup(t *testing.T) (socialEnv, store.Place, store.OutboundMessage) {
	t.Helper()
	e := newSocialEnv(t)
	ctx := context.Background()
	p := store.Place{ID: store.NewID(), WorkspaceID: e.ws.ID, UserID: anna.ID, Type: "posta", Label: "Posta"}
	env, err := e.keys.Encrypt([]byte(hookSecret), crypto.AAD("places", p.ID, posta.HookField))
	if err != nil {
		t.Fatal(err)
	}
	p.Meta = map[string]any{"source": "issued", posta.MetaHook: map[string]any{
		posta.MetaHookID: "hook-1", posta.MetaHookSecret: base64.StdEncoding.EncodeToString(env)}}
	if err := e.store.CreatePlace(ctx, &p); err != nil {
		t.Fatal(err)
	}
	m := store.OutboundMessage{WorkspaceID: e.ws.ID, UserID: anna.ID, IdempotencyKey: "run:b3:c1", Channel: "posta",
		Recipient: "place:" + p.ID, Body: new("Frisches Sauerteigbrot,\njeden Freitag"), Status: "sent", ProviderMessageID: new("post-1")}
	if err := e.store.CreateOutbound(ctx, &m); err != nil {
		t.Fatal(err)
	}
	return e, p, m
}

func (e socialEnv) deliver(t *testing.T, placeID, secret string, body any) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.url+"/api/posta/webhook/"+placeID, bytes.NewReader(raw))
	ts := "1795600000"
	req.Header.Set("X-Posta-Timestamp", ts)
	req.Header.Set("X-Posta-Signature", posta.Signature(secret, ts, raw))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct{ Outcome string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Outcome
}

func delivery(event, postID string, results ...map[string]any) map[string]any {
	return map[string]any{"id": "del-1", "event": event, "created_at": "2026-11-20T12:00:05.000Z",
		"data": map[string]any{"post": map[string]any{"id": postID, "status": "posted", "published_at": "2026-11-20T12:00:04.000Z"}, "results": results}}
}

func (e socialEnv) feed(t *testing.T) []store.FeedCard {
	t.Helper()
	cards, err := e.store.ListFeedCards(context.Background(), store.FeedFilter{WorkspaceID: e.ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	return cards
}

func TestPostaHookPublished(t *testing.T) {
	e, p, m := hookSetup(t)
	d := delivery("post.published", "post-1",
		map[string]any{"platform": "instagram", "status": "success", "error_code": nil, "error_message": nil},
		map[string]any{"platform": "bluesky", "status": "success"})
	if status, outcome := e.deliver(t, p.ID, hookSecret, d); status != 200 || outcome != "taken" {
		t.Fatalf("%d %s", status, outcome)
	}
	// Posta tells twice (it didn't see our answer): still one card.
	if status, outcome := e.deliver(t, p.ID, hookSecret, d); status != 200 || outcome != "taken" {
		t.Fatalf("again: %d %s", status, outcome)
	}
	cards := e.feed(t)
	if len(cards) != 1 {
		t.Fatalf("%d cards", len(cards))
	}
	// The workspace speaks German.
	c := cards[0]
	if c.Type != "result" || c.Title != "Dein Post ist raus" || *c.Body != "Auf Instagram und Bluesky. „Frisches Sauerteigbrot, jeden Freitag“" ||
		c.UserID != anna.ID || c.RunID != nil || c.Payload["social_post"] != "post-1" {
		t.Fatalf("card: %+v body %q", c, *c.Body)
	}
	got, _ := e.store.GetOutboundByKey(context.Background(), m.IdempotencyKey)
	if got.Status != "sent" || got.SentAt == nil || got.SentAt.UTC().Format("15:04:05") != "12:00:04" {
		t.Fatalf("outbound: %+v", got)
	}
}

func TestPostaHookFailed(t *testing.T) {
	e, p, m := hookSetup(t)
	d := delivery("post.failed", "post-1",
		map[string]any{"platform": "instagram", "status": "failed", "error_code": "MEDIA_REQUIRED", "error_message": "Instagram needs a picture"})
	if status, outcome := e.deliver(t, p.ID, hookSecret, d); status != 200 || outcome != "taken" {
		t.Fatalf("%d %s", status, outcome)
	}
	cards := e.feed(t)
	if len(cards) != 1 || cards[0].Type != "problem" || cards[0].Title != "Dein Post ist nicht rausgegangen" ||
		!strings.HasPrefix(*cards[0].Body, "Instagram: Instagram needs a picture\n„Frisches") {
		t.Fatalf("cards: %+v", cards)
	}
	got, _ := e.store.GetOutboundByKey(context.Background(), m.IdempotencyKey)
	if got.Status != "failed" || got.Error == nil || !strings.Contains(*got.Error, "needs a picture") {
		t.Fatalf("outbound: %+v", got)
	}
}

func TestPostaHookRefuses(t *testing.T) {
	e, p, _ := hookSetup(t)
	ok := delivery("post.published", "post-1")
	// Signed with another secret, or for an account that isn't there.
	if status, _ := e.deliver(t, p.ID, "whsec_other", ok); status != 401 {
		t.Fatalf("wrong secret: %d", status)
	}
	if status, _ := e.deliver(t, store.NewID(), hookSecret, ok); status != 404 {
		t.Fatalf("unknown account: %d", status)
	}
	if status, _ := e.deliver(t, "nope", hookSecret, ok); status != 404 {
		t.Fatalf("no id: %d", status)
	}
	// An account that never registered a webhook.
	bare := store.Place{ID: store.NewID(), WorkspaceID: e.ws2.ID, UserID: anna.ID, Type: "posta", Label: "Posta", Meta: map[string]any{"source": "pasted"}}
	if err := e.store.CreatePlace(context.Background(), &bare); err != nil {
		t.Fatal(err)
	}
	if status, _ := e.deliver(t, bare.ID, hookSecret, ok); status != 404 {
		t.Fatalf("no webhook: %d", status)
	}
	// A body changed on the way.
	raw, _ := json.Marshal(ok)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.url+"/api/posta/webhook/"+p.ID, bytes.NewReader(append(raw, ' ')))
	req.Header.Set("X-Posta-Timestamp", "1795600000")
	req.Header.Set("X-Posta-Signature", posta.Signature(hookSecret, "1795600000", raw))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("changed body: %d", resp.StatusCode)
	}
	// Signed, but nothing for the Feed: a ping, and a post made in Posta itself.
	if status, outcome := e.deliver(t, p.ID, hookSecret, map[string]any{"id": "d", "event": "ping"}); status != 200 || outcome != "ignored" {
		t.Fatalf("ping: %d %s", status, outcome)
	}
	if status, outcome := e.deliver(t, p.ID, hookSecret, delivery("post.published", "post-elsewhere")); status != 200 || outcome != "not_ours" {
		t.Fatalf("someone else's post: %d %s", status, outcome)
	}
	if n := len(e.feed(t)); n != 0 {
		t.Fatalf("%d cards", n)
	}
}

// Package do contains the 🟢 Do bricks: they act outside Laamo. Every
// action is written to outbound_messages before it happens, so a retried or
// reclaimed job never acts twice.
package do

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// ChatSender posts to a webhook (chat.Sender, or a fake).
type ChatSender interface {
	Send(ctx context.Context, placeType, webhook string, msg chat.Message) error
}

// ChatStore is what Post to chat needs from the store.
type ChatStore interface {
	store.PlaceStore
	store.OutboundStore
}

// Chat is the Post to chat brick (do.chat).
type Chat struct {
	Store  ChatStore
	Keys   *crypto.Keyring
	Sender ChatSender
}

const maxListed = 10

func (c Chat) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return in, nil
	}
	placeID := cfg.String("place")
	place, err := c.Store.GetPlace(ctx, placeID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && place.WorkspaceID != rc.Run.WorkspaceID) {
		return nil, queue.Permanent(errors.New("this chat isn't there anymore; pick another one in the brick"))
	}
	if err != nil {
		return nil, err
	}
	if place.Type != "discord" && place.Type != "slack" {
		return nil, queue.Permanent(fmt.Errorf("posting to %s isn't available yet", place.Type))
	}
	webhook, err := c.webhook(place)
	if err != nil {
		return nil, queue.Permanent(fmt.Errorf("the address of “%s” can't be read; paste it again under Me → Places", place.Label))
	}

	type msg struct {
		key  string
		text string
	}
	var msgs []msg
	prefix := header(rc)
	if cfg.String("mode") == "each" {
		for _, card := range in {
			msgs = append(msgs, msg{card.ID, prefix + cardText(card)})
		}
	} else {
		msgs = append(msgs, msg{"all", prefix + pileText(in)})
	}

	now := rc.Now
	if now == nil {
		now = time.Now
	}
	posted := map[string]bool{}
	for _, m := range msgs {
		sent, err := c.post(ctx, rc, place, webhook, m.key, m.text, now)
		if err != nil {
			return nil, err
		}
		posted[m.key] = sent
	}
	for i := range in {
		if posted["all"] || posted[in[i].ID] {
			in[i].Set("posted_at", now().UTC().Format(time.RFC3339), engine.TrustUser)
		}
	}
	return in, nil
}

// post sends one message at most once: outbound_messages first, then the
// webhook. On a retry, a message that may have gone out already is skipped.
func (c Chat) post(ctx context.Context, rc *engine.RunContext, place store.Place, webhook, key, text string, now func() time.Time) (bool, error) {
	idem := fmt.Sprintf("%s:%s:%s", rc.Run.ID, rc.BrickID, key)
	m := store.OutboundMessage{WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, RunID: &rc.Run.ID,
		IdempotencyKey: idem, Channel: place.Type, Recipient: "place:" + place.ID, Body: &text}
	err := c.Store.CreateOutbound(ctx, &m)
	if errors.Is(err, store.ErrConflict) {
		prev, gerr := c.Store.GetOutboundByKey(ctx, idem)
		if gerr != nil {
			return false, gerr
		}
		switch prev.Status {
		case "sent":
			return true, nil
		case "failed": // the chat said no last time: try again
			m = prev
		default: // pending: it may have gone out; never send twice
			rc.Log.Warn("chat message may have been sent by an earlier attempt; skipping", "key", idem)
			return false, nil
		}
	} else if err != nil {
		return false, err
	}

	err = c.Sender.Send(ctx, place.Type, webhook, chat.Message{Text: text})
	switch {
	case err == nil:
		sentAt := now().UTC()
		_, uerr := c.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: new("sent"), SentAt: &sentAt})
		return true, uerr
	case errors.Is(err, chat.ErrGone):
		_, _ = c.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: new("failed"), Error: new(err.Error())})
		return false, queue.Permanent(fmt.Errorf("the %s webhook for “%s” doesn't work anymore; paste a new one under Me → Places", title(place.Type), place.Label))
	case errors.Is(err, chat.ErrRejected):
		// The chat answered, so it surely wasn't delivered: a retry may send it.
		_, _ = c.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: new("failed"), Error: new(err.Error())})
	}
	// Other errors (timeouts): it may have been delivered, so the message
	// stays pending and a retry skips it.
	return false, fmt.Errorf("post to %s: %w", place.Type, err)
}

func (c Chat) webhook(p store.Place) (string, error) {
	if c.Keys == nil || p.SecretCiphertext == nil {
		return "", crypto.ErrNoKey
	}
	env, err := base64.StdEncoding.DecodeString(*p.SecretCiphertext)
	if err != nil {
		return "", err
	}
	b, err := c.Keys.Decrypt(env, crypto.AAD("places", p.ID, "secret"))
	return string(b), err
}

func title(t string) string {
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

func header(rc *engine.RunContext) string {
	h := "**" + rc.Helper.Name + "**\n"
	if rc.Helper.Name == "" {
		h = ""
	}
	if rc.TestMode {
		h = "🧪 Test · " + h
	}
	return h
}

func cut(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

func cardText(c engine.Card) string {
	var b strings.Builder
	title := c.Text("title")
	if title == "" {
		title = c.Text("name")
	}
	if title != "" {
		b.WriteString("**" + cut(title, 200) + "**\n")
	}
	for _, f := range []string{"summary", "body", "snippet", "text"} {
		if v := c.Text(f); v != "" {
			b.WriteString(cut(v, 600) + "\n")
			break
		}
	}
	if u := firstNonEmpty(c.Text("url"), c.Text("website"), c.Source); u != "" {
		b.WriteString(u)
	}
	return strings.TrimSpace(b.String())
}

func pileText(cards []engine.Card) string {
	if len(cards) == 1 {
		return cardText(cards[0])
	}
	var b strings.Builder
	for i, c := range cards {
		if i == maxListed {
			fmt.Fprintf(&b, "…and %d more", len(cards)-maxListed)
			break
		}
		title := cut(firstNonEmpty(c.Text("title"), c.Text("name"), c.Text("summary")), 150)
		if u := firstNonEmpty(c.Text("url"), c.Text("website"), c.Source); u != "" {
			fmt.Fprintf(&b, "• [%s](%s)\n", title, u)
		} else {
			fmt.Fprintf(&b, "• %s\n", title)
		}
	}
	return strings.TrimSpace(b.String())
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

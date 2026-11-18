// Package do contains the 🟢 Do bricks: they act outside Laamo. Every
// action is written to outbound_messages before it happens, so a retried or
// reclaimed job never acts twice.
package do

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/idna"

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
	de := rc.Workspace.Locale == "de"
	if cfg.String("mode") == "each" {
		for _, card := range in {
			msgs = append(msgs, msg{card.ID, prefix + cardText(card, de)})
		}
	} else {
		msgs = append(msgs, msg{"all", prefix + pileText(in, de)})
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
		h = "Test · " + h
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

func cardText(c engine.Card, de bool) string {
	var b strings.Builder
	if title := firstNonEmpty(c.Text("title"), c.Text("name"), c.Text("subject")); title != "" {
		b.WriteString("**" + cut(plain(title), 200) + "**\n")
	}
	if o := origin(c, de); o != "" {
		b.WriteString(o + "\n")
	}
	// A draft from Write it is shown whole (Discord allows 2000 characters).
	n := 600
	if c.Kind == "draft" {
		n = 1500
	}
	for _, f := range []string{"summary", "body", "snippet", "text"} {
		if v := c.Text(f); v != "" {
			b.WriteString(cut(plain(v), n) + "\n")
			break
		}
	}
	if u, _ := link(firstNonEmpty(c.Text("url"), c.Text("website"), c.Source)); u != "" {
		b.WriteString(u) // shown whole: the reader sees where it goes
	}
	return strings.TrimSpace(b.String())
}

func pileText(cards []engine.Card, de bool) string {
	if len(cards) == 1 {
		return cardText(cards[0], de)
	}
	var b strings.Builder
	for i, c := range cards {
		if i == maxListed {
			fmt.Fprintf(&b, "…and %d more", len(cards)-maxListed)
			break
		}
		title := cut(plain(firstNonEmpty(c.Text("title"), c.Text("name"), c.Text("summary"))), 150)
		if u, host := link(firstNonEmpty(c.Text("url"), c.Text("website"), c.Source)); u != "" {
			// The host stays visible: the title alone could say anything.
			fmt.Fprintf(&b, "• [%s](%s) · %s", title, u, host)
		} else {
			fmt.Fprintf(&b, "• %s", title)
		}
		if o := origin(c, de); o != "" {
			b.WriteString(" · " + o)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// Prompt-injection safety: what a page or an email says, and what the brain
// wrote about it, goes into a chat message as plain text. It can't become a
// link whose text hides where it goes ("[Your bank](https://evil…)"):
// links come only from a card's link fields (link), with the host shown.
var urlInText = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>()\[\]"'` + "`" + `]+`)

// plain makes card text safe to post: a URL in it becomes "(link: host)",
// and what's left of link syntax ([text](…), Slack's <url|text>) breaks.
func plain(s string) string {
	s = urlInText.ReplaceAllStringFunc(s, func(u string) string {
		if h := hostOf(u); h != "" {
			return "(link: " + h + ")"
		}
		return "(link)"
	})
	return strings.NewReplacer("](", "] (", "<", "‹", ">", "›").Replace(s)
}

// link checks a card's link field: only http(s), and escaped so it can't
// end a markdown or Slack link early. It returns the URL and its host.
func link(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.ContainsAny(raw, " \t\n") {
		return "", ""
	}
	h := hostOf(raw)
	if h == "" {
		return "", ""
	}
	return strings.NewReplacer("(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "|", "%7C").Replace(raw), h
}

// hostOf is a URL's host as people should read it: lower case, without
// www., and in its ASCII form, so a look-alike (раypal.com) shows as xn--….
func hostOf(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		h = a
	}
	return h
}

// origin says who an email card came from and whether Laamo could check
// the sender (the domain signed the email), so a message that only claims
// to be from "your bank" reads as such.
func origin(c engine.Card, de bool) string {
	from := c.Text("from")
	if from == "" || c.TrustOf("from") != engine.TrustUntrusted {
		return "" // not an email the helper received
	}
	who := plain(from)
	if name := strings.TrimSpace(c.Text("from_name")); name != "" {
		who = cut(plain(name), 60) + " (" + who + ")"
	}
	checked := c.Text("from_signed") == "yes" && c.TrustOf("from_signed") == engine.TrustUser
	switch {
	case de && checked:
		return "✉️ Von " + who + " · Absender geprüft"
	case de:
		return "✉️ Von " + who + " · Absender nicht geprüft"
	case checked:
		return "✉️ From " + who + " · sender checked"
	}
	return "✉️ From " + who + " · sender not checked"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

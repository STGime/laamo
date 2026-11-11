package do_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

const webhook = "https://discord.com/api/webhooks/123/SECRETtoken"

type sent struct{ typ, url, text string }

type fakeSender struct {
	sent []sent
	errs []error // returned in order, then nil
}

func (f *fakeSender) Send(_ context.Context, typ, url string, m chat.Message) error {
	f.sent = append(f.sent, sent{typ, url, m.Text})
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return err
	}
	return nil
}

type env struct {
	st     *memory.Store
	keys   *crypto.Keyring
	sender *fakeSender
	rc     *engine.RunContext
	place  store.Place
}

func setup(t *testing.T, placeType string) *env {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	keys, err := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(k), "")
	if err != nil {
		t.Fatal(err)
	}
	st := memory.New()
	rc := brickstest.RunContext("do.chat")
	rc.Helper = store.Helper{Name: "Morning Briefing"}
	p := store.Place{ID: store.NewID(), WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, Type: placeType, Label: "#news"}
	sealed, err := keys.Encrypt([]byte(webhook), crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	p.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := st.CreatePlace(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
	return &env{st: st, keys: keys, sender: &fakeSender{}, rc: rc, place: p}
}

func (e *env) brick() do.Chat { return do.Chat{Store: e.st, Keys: e.keys, Sender: e.sender} }

func (e *env) run(t *testing.T, in []engine.Card, cfg map[string]any) ([]engine.Card, error) {
	t.Helper()
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg["place"] = e.place.ID
	return brickstest.Contract(t, "do.chat", e.brick(), e.rc, in, cfg)
}

func cards(n int) []engine.Card {
	out := make([]engine.Card, n)
	for i := range out {
		out[i] = engine.Card{ID: fmt.Sprintf("c%d", i), Kind: "web_result"}
		out[i].Set("title", fmt.Sprintf("News %d", i), engine.TrustUntrusted)
		out[i].Set("url", fmt.Sprintf("https://x.de/%d", i), engine.TrustUntrusted)
	}
	return out
}

func summary() []engine.Card {
	c := engine.Card{ID: "s1", Kind: "summary"}
	c.Set("summary", "Flour gets pricier.", engine.TrustExtracted)
	return []engine.Card{c}
}

func TestPostOneMessage(t *testing.T) {
	e := setup(t, "discord")
	out, err := e.run(t, summary(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.sender.sent) != 1 || e.sender.sent[0].url != webhook || e.sender.sent[0].text != "**Morning Briefing**\nFlour gets pricier." {
		t.Fatalf("sent: %+v", e.sender.sent)
	}
	if out[0].Text("posted_at") == "" {
		t.Fatal("posted_at not set")
	}
	m, err := e.st.GetOutboundByKey(context.Background(), e.rc.Run.ID+":b1:all")
	if err != nil || m.Status != "sent" || m.Recipient != "place:"+e.place.ID || strings.Contains(m.Recipient, "SECRET") {
		t.Fatalf("outbound: %+v %v", m, err)
	}

	// A list of 12 cards: linked titles, cut at 10.
	e2 := setup(t, "discord")
	if _, err := e2.run(t, cards(12), nil); err != nil {
		t.Fatal(err)
	}
	text := e2.sender.sent[0].text
	if !strings.Contains(text, "• [News 0](https://x.de/0)") || !strings.Contains(text, "…and 2 more") || strings.Contains(text, "News 10") {
		t.Fatalf("list: %s", text)
	}
}

func TestPostEachAndTestMode(t *testing.T) {
	e := setup(t, "slack")
	e.rc.TestMode = true
	if _, err := e.run(t, cards(3), map[string]any{"mode": "each"}); err != nil {
		t.Fatal(err)
	}
	if len(e.sender.sent) != 3 || e.sender.sent[0].typ != "slack" || !strings.HasPrefix(e.sender.sent[0].text, "🧪 Test · **Morning Briefing**") {
		t.Fatalf("sent: %+v", e.sender.sent)
	}
}

func TestPlaceProblems(t *testing.T) {
	e := setup(t, "discord")
	other := store.Place{WorkspaceID: store.NewID(), Type: "discord", Label: "theirs", SecretCiphertext: e.place.SecretCiphertext}
	_ = e.st.CreatePlace(context.Background(), &other)
	if _, err := brickstest.Contract(t, "do.chat", e.brick(), e.rc, summary(), map[string]any{"place": other.ID}); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("another workspace's place: %v", err)
	}
	// Ciphertext copied from another place doesn't decrypt (bound to its row).
	copied := store.Place{WorkspaceID: e.rc.Run.WorkspaceID, Type: "discord", Label: "copy", SecretCiphertext: e.place.SecretCiphertext}
	_ = e.st.CreatePlace(context.Background(), &copied)
	if _, err := brickstest.Contract(t, "do.chat", e.brick(), e.rc, summary(), map[string]any{"place": copied.ID}); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("moved ciphertext: %v", err)
	}
	tg := setup(t, "telegram")
	if _, err := tg.run(t, summary(), nil); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("telegram: %v", err)
	}
	if len(e.sender.sent)+len(tg.sender.sent) != 0 {
		t.Fatal("nothing may be sent")
	}
}

func TestNeverSendsTwice(t *testing.T) {
	t.Run("webhook gone", func(t *testing.T) {
		e := setup(t, "discord")
		e.sender.errs = []error{fmt.Errorf("%w (HTTP 404)", chat.ErrGone)}
		_, err := e.run(t, summary(), nil)
		if !errors.Is(err, queue.ErrPermanent) || !strings.Contains(err.Error(), "paste a new one") {
			t.Fatalf("gone: %v", err)
		}
	})
	t.Run("rejected, then sent on retry", func(t *testing.T) {
		e := setup(t, "discord")
		e.sender.errs = []error{fmt.Errorf("%w: HTTP 429", chat.ErrRejected)}
		if _, err := e.run(t, summary(), nil); err == nil || errors.Is(err, queue.ErrPermanent) {
			t.Fatalf("rejection should be retried: %v", err)
		}
		if _, err := e.run(t, summary(), nil); err != nil {
			t.Fatal(err)
		}
		m, _ := e.st.GetOutboundByKey(context.Background(), e.rc.Run.ID+":b1:all")
		if len(e.sender.sent) != 2 || m.Status != "sent" {
			t.Fatalf("resend after rejection: %d sends, %s", len(e.sender.sent), m.Status)
		}
	})
	t.Run("timeout: maybe delivered, skipped on retry", func(t *testing.T) {
		e := setup(t, "discord")
		e.sender.errs = []error{errors.New("chat: context deadline exceeded")}
		if _, err := e.run(t, summary(), nil); err == nil {
			t.Fatal("want the timeout")
		}
		if _, err := e.run(t, summary(), nil); err != nil {
			t.Fatal(err)
		}
		if len(e.sender.sent) != 1 {
			t.Fatalf("a maybe-delivered message must not be sent again: %d sends", len(e.sender.sent))
		}
	})
	t.Run("already sent", func(t *testing.T) {
		e := setup(t, "discord")
		_, _ = e.run(t, cards(2), map[string]any{"mode": "each"})
		out, err := e.run(t, cards(2), map[string]any{"mode": "each"})
		if err != nil || len(e.sender.sent) != 2 || out[1].Text("posted_at") == "" {
			t.Fatalf("rerun after success: %v, %d sends", err, len(e.sender.sent))
		}
	})
}

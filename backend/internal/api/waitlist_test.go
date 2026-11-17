package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
	lmail "github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/proof"
	"github.com/STGime/laamo/backend/internal/store/memory"
	"github.com/STGime/laamo/backend/internal/waitlist"
)

type outbox struct {
	mu   sync.Mutex
	sent []lmail.Outgoing
}

func (o *outbox) Send(_ context.Context, m lmail.Outgoing) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, m)
	return nil
}

func TestWaitlist(t *testing.T) {
	st := memory.New()
	box := &outbox{}
	wl := &waitlist.Service{Store: st, Mail: box, SiteURL: "https://laamo.app"}
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: config.Config{DemoSecret: "k"}, Store: st, Auth: fakeAuth{},
		Waitlist: wl}).Handler())
	t.Cleanup(srv.Close)
	c := env{url: srv.URL, store: st}.client(t, "") // no account
	ctx := context.Background()

	join := func(body api.WaitlistJoin) *api.JoinWaitlistResponse {
		t.Helper()
		if body.Token == "" {
			ch := must(c.GetWaitlistChallengeWithResponse(ctx))
			if ch.StatusCode() != http.StatusOK {
				t.Fatalf("challenge: %d %s", ch.StatusCode(), ch.Body)
			}
			body.Token, body.Nonce = ch.JSON200.Token, proof.Solve(proof.Challenge{Token: ch.JSON200.Token, Bits: ch.JSON200.Bits})
		}
		return must(c.JoinWaitlistWithResponse(ctx, body))
	}

	de := api.De
	interests := []api.WaitlistJoinInterests{api.Social, api.Customers}
	res := join(api.WaitlistJoin{Email: "Anna@Example.com", Lang: &de, Interests: &interests})
	if res.StatusCode() != http.StatusAccepted || res.JSON202.Status != api.CheckInbox {
		t.Fatalf("join: %d %s", res.StatusCode(), res.Body)
	}
	if len(box.sent) != 1 || box.sent[0].To != "anna@example.com" {
		t.Fatalf("email: %+v", box.sent)
	}

	// Confirm with the link's token.
	link := box.sent[0].Text[strings.Index(box.sent[0].Text, "https://"):]
	link = link[:strings.IndexByte(link, '\n')]
	u, _ := url.Parse(link)
	if u.Path != "/de/waitlist/confirm" {
		t.Fatalf("link: %s", link)
	}
	conf := must(c.ConfirmWaitlistWithResponse(ctx, api.ConfirmWaitlistJSONRequestBody{Token: u.Query().Get("t")}))
	if conf.StatusCode() != http.StatusOK || len(conf.JSON200.RefCode) != 8 || conf.JSON200.Lang != api.De {
		t.Fatalf("confirm: %d %s", conf.StatusCode(), conf.Body)
	}
	again := must(c.ConfirmWaitlistWithResponse(ctx, api.ConfirmWaitlistJSONRequestBody{Token: u.Query().Get("t")}))
	if again.StatusCode() != http.StatusBadRequest {
		t.Fatalf("link used twice: %d", again.StatusCode())
	}

	// Not an email.
	if r := join(api.WaitlistJoin{Email: "anna"}); r.StatusCode() != http.StatusBadRequest {
		t.Fatalf("bad email: %d", r.StatusCode())
	}
	// A bot fills the hidden field: same answer, no email.
	bot := "https://spam.example"
	if r := join(api.WaitlistJoin{Email: "bot@example.com", Website: &bot}); r.StatusCode() != http.StatusAccepted {
		t.Fatalf("honeypot: %d", r.StatusCode())
	}
	// No or a reused proof of work.
	if r := join(api.WaitlistJoin{Email: "carl@example.com", Token: "1.2.x", Nonce: "1"}); r.StatusCode() != http.StatusBadRequest {
		t.Fatalf("bad proof: %d", r.StatusCode())
	}
	if len(box.sent) != 1 {
		t.Fatalf("emails after the refused sign-ups: %d", len(box.sent))
	}
	if _, err := st.WaitlistByEmail(ctx, "bot@example.com"); err == nil {
		t.Fatal("the bot is on the list")
	}

	// Five sign-ups per IP and 10 minutes; the sixth waits.
	join(api.WaitlistJoin{Email: "dora@example.com"})
	if r := join(api.WaitlistJoin{Email: "emil@example.com"}); r.StatusCode() != http.StatusTooManyRequests {
		t.Fatalf("sixth sign-up: %d", r.StatusCode())
	}
}

func TestWaitlistClosed(t *testing.T) {
	e := newEnv(t, config.Config{})
	c := e.client(t, "")
	ctx := context.Background()
	ch := must(c.GetWaitlistChallengeWithResponse(ctx))
	nonce := proof.Solve(proof.Challenge{Token: ch.JSON200.Token, Bits: ch.JSON200.Bits})
	r := must(c.JoinWaitlistWithResponse(ctx, api.WaitlistJoin{Email: "anna@example.com", Token: ch.JSON200.Token, Nonce: nonce}))
	if r.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("closed list: %d", r.StatusCode())
	}
}

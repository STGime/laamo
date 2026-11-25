package posta_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/posta/postatest"
)

func TestPartner(t *testing.T) {
	f, svc := postatest.New(t)
	ctx := context.Background()
	p := svc.Partner
	in := posta.NewAccount{Email: "anna@example.com", ExternalUserID: "user-1", DisplayName: "Bäckerei", TermsAcceptedAt: time.Now()}

	a, err := p.CreateAccount(ctx, in)
	if err != nil || !a.Created || a.Trial != "started" || a.APIToken == "" || !a.Plan.IsActive || *a.Plan.DaysRemaining != 5 {
		t.Fatalf("create: %+v %v", a, err)
	}
	// Creating again returns the account with a NEW token.
	b, err := p.CreateAccount(ctx, in)
	if err != nil || b.Created || b.Trial != "existing" || b.APIToken == a.APIToken {
		t.Fatalf("create again: %+v %v", b, err)
	}
	if _, err := svc.As(a.APIToken).Channels(ctx); !errors.Is(err, posta.ErrUnauthorized) {
		t.Fatalf("the old token still works: %v", err)
	}
	if cs, err := svc.As(b.APIToken).Channels(ctx); err != nil || len(cs) != 0 {
		t.Fatalf("channels: %v %v", cs, err)
	}

	until := time.Now().Add(30 * 24 * time.Hour)
	if got, err := p.SetPlan(ctx, "user-1", "starter", &until); err != nil || got.Plan.Type != "starter" || !got.Plan.IsActive {
		t.Fatalf("set plan: %+v %v", got, err)
	}
	if got, err := p.GetAccount(ctx, "user-1"); err != nil || got.Plan.Type != "starter" || got.APIToken != "" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if token, err := p.RotateToken(ctx, "user-1"); err != nil || token == b.APIToken || token != f.Token("user-1") {
		t.Fatalf("rotate: %q %v", token, err)
	}

	// The email of someone's own Posta account: no account, no token.
	f.OwnAccount("ben@example.com")
	_, err = p.CreateAccount(ctx, posta.NewAccount{Email: "ben@example.com", ExternalUserID: "user-2", TermsAcceptedAt: time.Now()})
	var pe *posta.Error
	if !errors.Is(err, posta.ErrAccountExists) || !errors.As(err, &pe) || pe.Reason != "account_exists" {
		t.Fatalf("own account: %v", err)
	}
	_, err = p.CreateAccount(ctx, posta.NewAccount{Email: "other@example.com", ExternalUserID: "user-1", TermsAcceptedAt: time.Now()})
	if !errors.Is(err, posta.ErrEmailMismatch) {
		t.Fatalf("other email: %v", err)
	}

	if _, err := p.GetAccount(ctx, "nobody"); !errors.Is(err, posta.ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	if err := p.DeleteAccount(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteAccount(ctx, "user-1"); err != nil { // gone already: fine
		t.Fatal(err)
	}
	if u, err := p.Usage(ctx); err != nil || u.AccountsTotal != 0 {
		t.Fatalf("usage: %+v %v", u, err)
	}
}

// A key Posta doesn't know, and errors that must not leak it.
func TestKeysStayOutOfErrors(t *testing.T) {
	_, svc := postatest.New(t)
	ctx := context.Background()
	bad := &posta.Partner{Key: "postapk_secret_value", URL: svc.Partner.URL}
	_, err := bad.Usage(ctx)
	if !errors.Is(err, posta.ErrUnauthorized) || strings.Contains(err.Error(), "postapk_secret_value") {
		t.Fatalf("wrong key: %v", err)
	}
	gone := &posta.Partner{Key: "postapk_secret_value", URL: "http://127.0.0.1:1"}
	if _, err := gone.Usage(ctx); err == nil || strings.Contains(err.Error(), "postapk_secret_value") {
		t.Fatalf("unreachable: %v", err)
	}
}

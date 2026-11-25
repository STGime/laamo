package posta_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

// Posta's real answers are looser than its documentation.
func TestChannelShapes(t *testing.T) {
	for name, body := range map[string]string{
		"as the guide says": `{"accounts":[{"id":"123","platform":"instagram","username":"anna","displayName":"Anna","isActive":true,"connectionError":null}]}`,
		"snake case":        `{"accounts":[{"id":"123","platform":"instagram","username":"anna","display_name":"Anna","is_active":true}]}`,
		"a number as id":    `{"accounts":[{"id":123,"platform":"instagram","username":"anna","displayName":"Anna"}]}`,
		"a bare list":       `[{"id":"123","platform":"instagram","username":"anna","displayName":"Anna"}]`,
		"under data":        `{"data":[{"id":"123","platform":"instagram","username":null,"displayName":"Anna","profileImageUrl":null}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		cs, err := (&posta.User{Token: "posta_x", URL: srv.URL}).Channels(context.Background())
		srv.Close()
		if err != nil || len(cs) != 1 || cs[0].ID != "123" || cs[0].Platform != "instagram" || cs[0].DisplayName != "Anna" || !cs[0].IsActive {
			t.Errorf("%s: %+v %v", name, cs, err)
		}
	}
	// A broken channel: the reason may be text or an object.
	for _, body := range []string{
		`{"accounts":[{"id":"1","platform":"x","isActive":false,"connectionError":"token expired"}]}`,
		`{"accounts":[{"id":"1","platform":"x","is_active":false,"connection_error":{"code":"EXPIRED","message":"token expired"}}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		cs, err := (&posta.User{Token: "posta_x", URL: srv.URL}).Channels(context.Background())
		srv.Close()
		if err != nil || len(cs) != 1 || cs[0].IsActive || cs[0].ConnectionError != "token expired" {
			t.Errorf("%s: %+v %v", body, cs, err)
		}
	}
	// Something else entirely: the error names the fields, never the values.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"channels":"secret-value","total":1}`))
	}))
	defer srv.Close()
	_, err := (&posta.User{Token: "posta_x", URL: srv.URL}).Channels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "channels, total") || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unknown shape: %v", err)
	}
}

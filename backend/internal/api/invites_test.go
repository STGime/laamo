package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/store"
)

func TestInviteOnly(t *testing.T) {
	e := newEnv(t, config.Config{InviteOnly: true})
	ctx := context.Background()
	now := time.Now().UTC()
	// Anna was invited (the address as she typed it on the site, lower-cased).
	if err := e.store.AddWaitlist(ctx, &store.WaitlistEntry{Email: strings.ToLower(anna.Email), RefCode: "anna2345",
		Consent: "waitlist-v1", ConfirmedAt: &now, InvitedAt: &now}); err != nil {
		t.Fatal(err)
	}
	// Ben is on the list but not invited yet.
	if err := e.store.AddWaitlist(ctx, &store.WaitlistEntry{Email: strings.ToLower(ben.Email), RefCode: "benb2345",
		Consent: "waitlist-v1", ConfirmedAt: &now}); err != nil {
		t.Fatal(err)
	}

	if res := must(e.client(t, "anna").GetMeWithResponse(ctx)); res.StatusCode() != http.StatusOK {
		t.Fatalf("invited: %d %s", res.StatusCode(), res.Body)
	}
	res := must(e.client(t, "ben").GetMeWithResponse(ctx))
	if res.StatusCode() != http.StatusForbidden || res.JSON403 == nil || res.JSON403.Code != api.ErrorCodeNotInvited {
		t.Fatalf("not invited: %d %s", res.StatusCode(), res.Body)
	}
	// Public endpoints stay open; no token is still 401, not 403.
	if h := must(e.client(t, "").GetHealthWithResponse(ctx)); h.StatusCode() != http.StatusOK {
		t.Fatalf("health: %d", h.StatusCode())
	}
	if r := must(e.client(t, "").GetMeWithResponse(ctx)); r.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("no token: %d", r.StatusCode())
	}
}

func TestInviteOnlyOff(t *testing.T) {
	e := newEnv(t, config.Config{})
	if res := must(e.client(t, "ben").GetMeWithResponse(context.Background())); res.StatusCode() != http.StatusOK {
		t.Fatalf("open app: %d %s", res.StatusCode(), res.Body)
	}
}

package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/demo"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// The demo needs no account; without a demo service it says it's off.
func TestDemoIsPublic(t *testing.T) {
	ctx := context.Background()
	off := newEnv(t, config.Config{}).client(t, "")
	if r := must(off.RunDemoWithResponse(ctx, api.DemoRequest{Template: "weekend-scout", City: "Leipzig", Token: "t", Nonce: "1"})); r.StatusCode() != 503 {
		t.Fatalf("demo off: %d %s", r.StatusCode(), r.Body)
	}

	srv := httptest.NewServer(api.NewServer(api.Deps{Store: memory.New(), Auth: fakeAuth{},
		Demo: &demo.Service{Catalog: catalog.Default()}}).Handler())
	t.Cleanup(srv.Close)
	c := env{url: srv.URL}.client(t, "")
	ch := must(c.GetDemoChallengeWithResponse(ctx))
	if ch.StatusCode() != 200 || ch.JSON200.Token == "" || ch.JSON200.Bits < 1 {
		t.Fatalf("challenge: %d %s", ch.StatusCode(), ch.Body)
	}
	// No work done: refused before anything runs.
	r := must(c.RunDemoWithResponse(ctx, api.DemoRequest{Template: "weekend-scout", City: "Leipzig", Token: ch.JSON200.Token, Nonce: "no"}))
	if r.StatusCode() != 400 {
		t.Fatalf("no proof: %d %s", r.StatusCode(), r.Body)
	}
}

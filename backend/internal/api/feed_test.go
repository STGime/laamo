package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// A run pauses at Check first; the user reads the card through the API,
// decides, and the run goes on with the decided pile.
func TestFeedDecisionResumesRun(t *testing.T) {
	st := memory.New()
	var resumed []string
	srv := httptest.NewServer(api.NewServer(api.Deps{
		Config: config.Config{}, Store: st, Auth: fakeAuth{"anna": anna, "ben": ben},
		Resume: func(_ context.Context, runID string) error { resumed = append(resumed, runID); return nil },
	}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}

	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Leads", Status: "active"})
	stack, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: map[string]any{"query": "Bäckerei Leipzig"}},
		{ID: "b3", Type: "ask.check"},
		{ID: "b4", Type: "do.chat", Config: map[string]any{"place": "p1"}},
	}})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 1, Stack: stack})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)

	var posted []engine.Card
	eng := &engine.Engine{Store: st, Catalog: catalog.Default(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Bricks: map[string]engine.Brick{
			"when.tap": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
				return nil, nil
			}),
			"look.search": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
				var out []engine.Card
				for _, title := range []string{"Mehlpreise steigen", "Fußball", "Weihnachtsmarkt"} {
					var c engine.Card
					c.Set("title", title, engine.TrustUntrusted)
					out = append(out, c)
				}
				return out, nil
			}),
			"do.chat": engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
				posted = append(posted, in...)
				return in, nil
			}),
		}}
	ctx := context.Background()
	r, err := eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = st.GetRun(ctx, r.ID)
	if r.Status != store.RunWaiting {
		t.Fatalf("run %s", r.Status)
	}
	fcID := openapi_types.UUID{}
	_ = fcID.UnmarshalText([]byte(*r.WaitingOnCardID))

	// Ben isn't a member: 404, for reading and deciding.
	if res := must(e.client(t, "ben").GetFeedCardWithResponse(ctx, fcID)); res.StatusCode() != http.StatusNotFound {
		t.Fatalf("ben reads: %d", res.StatusCode())
	}
	anna := e.client(t, "anna")
	got := must(anna.GetFeedCardWithResponse(ctx, fcID))
	if got.StatusCode() != http.StatusOK || got.JSON200.Type != "approval" || len(got.JSON200.Cards) != 3 ||
		got.JSON200.Cards[0].Fields["title"] != "Mehlpreise steigen" || got.JSON200.Cards[0].Trust["title"] != "untrusted" {
		t.Fatalf("get: %d %s", got.StatusCode(), got.Body)
	}
	if *got.JSON200.Body != "Bitte prüfe 3, bevor es weitergeht." {
		t.Fatalf("body %q", *got.JSON200.Body)
	}
	cards := got.JSON200.Cards

	bad := must(anna.DecideFeedCardWithResponse(ctx, fcID, api.FeedDecision{State: "answered", Answer: new("ja")}))
	if bad.StatusCode() != http.StatusBadRequest {
		t.Fatalf("wrong decision for a check: %d %s", bad.StatusCode(), bad.Body)
	}

	edits := map[string]map[string]string{cards[2].Id.String(): {"title": "Weihnachtsmarkt Leipzig"}}
	res := must(anna.DecideFeedCardWithResponse(ctx, fcID, api.FeedDecision{State: "edited",
		Skip: &[]openapi_types.UUID{cards[1].Id}, Edits: &edits}))
	if res.StatusCode() != http.StatusOK || res.JSON200.State != "edited" || res.JSON200.DecidedAt == nil {
		t.Fatalf("decide: %d %s", res.StatusCode(), res.Body)
	}
	if len(resumed) != 1 || resumed[0] != r.ID {
		t.Fatalf("resumed %v", resumed)
	}
	again := must(anna.DecideFeedCardWithResponse(ctx, fcID, api.FeedDecision{State: "approved"}))
	if again.StatusCode() != http.StatusConflict {
		t.Fatalf("second decision: %d", again.StatusCode())
	}

	// The job the API queued runs.
	if err := eng.Execute(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = st.GetRun(ctx, r.ID)
	if r.Status != store.RunDone || len(posted) != 2 || posted[1].Text("title") != "Weihnachtsmarkt Leipzig" ||
		posted[1].TrustOf("title") != engine.TrustUser {
		t.Fatalf("run %s, posted %+v", r.Status, posted)
	}
}

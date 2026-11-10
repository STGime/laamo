package engine_test

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
)

// TestLiveRun runs the fake Morning Briefing against a real Eurobase
// project: runs and steps via REST, the pile via insert_cards, the Feed card.
// Everything it creates hangs off one helper, which is deleted at the end
// (the foreign keys cascade). Needs LAAMO_LIVE_USER_ID: an end user of the
// project (e.g. from scripts/dev-token.sh); the test creates a workspace
// for them only if they have none.
func TestLiveRun(t *testing.T) {
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || os.Getenv("LAAMO_LIVE_USER_ID") == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and LAAMO_LIVE_USER_ID to run against Eurobase")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	user := os.Getenv("LAAMO_LIVE_USER_ID")
	c := eb.New(cmp.Or(os.Getenv("EUROBASE_URL"), "https://laamo.eurobase.app"), os.Getenv("EUROBASE_PRIVATE_KEY"))
	st := sebs.New(c)

	members, err := st.Memberships(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	var wsID string
	if len(members) > 0 {
		wsID = members[0].WorkspaceID
	} else {
		ws, err := st.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: user, Name: "Live test"})
		if err != nil {
			t.Fatal(err)
		}
		wsID = ws.ID
	}

	helper := store.Helper{ID: store.NewID(), WorkspaceID: wsID, UserID: user, Name: "Live test helper", Status: "active"}
	if err := c.Insert(ctx, "helpers", helper.ID, helper, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), "helpers", helper.ID) })
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: morningBriefing()})
	version := store.HelperVersion{ID: store.NewID(), WorkspaceID: wsID, UserID: user, HelperID: helper.ID, Version: 1, Stack: raw}
	if err := c.Insert(ctx, "helper_versions", version.ID, version, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Update(ctx, "helpers", helper.ID, map[string]string{"current_version_id": version.ID}, nil); err != nil {
		t.Fatal(err)
	}

	calls := &calls{}
	e := &engine.Engine{Store: st, Catalog: catalog.Default(), Bricks: fakeBricks(calls, 5),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r, err := e.StartRun(ctx, engine.StartRequest{HelperID: helper.ID, TriggerType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Execute(ctx, r.ID); err != nil {
		t.Fatal(err)
	}

	r, err = st.GetRun(ctx, r.ID)
	if err != nil || r.Status != store.RunDone || r.NextBrickIndex != 5 {
		t.Fatalf("run: %+v %v", r, err)
	}
	steps, err := st.ListSteps(ctx, r.ID)
	if err != nil || len(steps) != 5 {
		t.Fatalf("steps: %d %v", len(steps), err)
	}
	cards, err := st.ListCards(ctx, steps[2].ID)
	if err != nil || len(cards) != 3 || cards[0].Score == nil || cards[0].Trust["title"] != engine.TrustUntrusted {
		t.Fatalf("kept cards: %+v %v", cards, err)
	}
	var feed []store.FeedCard
	if _, err := c.List(ctx, "feed_cards", eb.NewQuery().Eq("run_id", r.ID), &feed); err != nil || len(feed) != 1 || feed[0].Type != "result" {
		t.Fatalf("feed: %+v %v", feed, err)
	}
	t.Logf("run %s: %d steps, result %q", r.ID, len(steps), *feed[0].Body)
}

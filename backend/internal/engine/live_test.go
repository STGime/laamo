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
	"github.com/STGime/laamo/backend/internal/engine"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
)

// liveHelper creates a helper with this stack for LAAMO_LIVE_USER_ID on the
// real project; it's deleted (with its runs) when the test ends.
func liveHelper(t *testing.T, stack []sb) (context.Context, *eb.Client, *sebs.Store, string) {
	t.Helper()
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || os.Getenv("LAAMO_LIVE_USER_ID") == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and LAAMO_LIVE_USER_ID to run against Eurobase")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
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
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: stack})
	version := store.HelperVersion{ID: store.NewID(), WorkspaceID: wsID, UserID: user, HelperID: helper.ID, Version: 1, Stack: raw}
	if err := c.Insert(ctx, "helper_versions", version.ID, version, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Update(ctx, "helpers", helper.ID, map[string]string{"current_version_id": version.ID}, nil); err != nil {
		t.Fatal(err)
	}

	return ctx, c, st, helper.ID
}

// TestLiveRun runs the fake Morning Briefing against a real Eurobase
// project: runs and steps via REST, the pile via insert_cards, the Feed card.
// Everything it creates hangs off one helper, which is deleted at the end
// (the foreign keys cascade). Needs LAAMO_LIVE_USER_ID: an end user of the
// project (e.g. from scripts/dev-token.sh); the test creates a workspace
// for them only if they have none.
func TestLiveRun(t *testing.T) {
	ctx, c, st, helperID := liveHelper(t, morningBriefing())

	calls := &calls{}
	e := &engine.Engine{Store: st, Catalog: catalog.Default(), Bricks: fakeBricks(calls, 5),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r, err := e.StartRun(ctx, engine.StartRequest{HelperID: helperID, TriggerType: "manual"})
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

// TestLivePauseResume pauses at Check first on the real project (Feed card
// with the step's id, run waiting on it), decides, and checks that
// resume_waiting_runs queues the run before the engine finishes it.
func TestLivePauseResume(t *testing.T) {
	ctx, c, st, helperID := liveHelper(t, checkStack("skip"))
	calls := &calls{}
	e := &engine.Engine{Store: st, Catalog: catalog.Default(), Bricks: fakeBricks(calls, 4),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r, err := e.StartRun(ctx, engine.StartRequest{HelperID: helperID, TriggerType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Execute(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if r, err = st.GetRun(ctx, r.ID); err != nil || r.Status != store.RunWaiting || r.WaitingOnCardID == nil {
		t.Fatalf("run: %+v %v", r, err)
	}
	fc, err := st.GetFeedCard(ctx, *r.WaitingOnCardID)
	if err != nil || fc.StepID == nil || *fc.StepID != fc.ID || fc.Type != engine.FeedApproval {
		t.Fatalf("feed card: %+v %v", fc, err)
	}
	shown, err := st.ListCards(ctx, fc.ID)
	if err != nil || len(shown) != 4 {
		t.Fatalf("shown: %d %v", len(shown), err)
	}

	var res struct{ Enqueued int }
	if err := c.RPC(ctx, "resume_waiting_runs", map[string]any{"p_run": r.ID}, &res); err != nil || res.Enqueued != 0 {
		t.Fatalf("open card resumed: %+v %v", res, err)
	}
	state := engine.StateApproved
	if _, err := st.UpdateFeedCard(ctx, fc.ID, store.FeedCardPatch{State: &state,
		Decision: map[string]any{"skip": []any{shown[0].CardID}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.RPC(ctx, "resume_waiting_runs", map[string]any{"p_run": r.ID}, &res); err != nil || res.Enqueued != 1 {
		t.Fatalf("decided card not resumed: %+v %v", res, err)
	}
	var jobs []struct {
		ID      string `json:"id"`
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if _, err := c.List(ctx, "jobs", eb.NewQuery().Eq("kind", "run").Eq("status", "queued"), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs { // the test runs the engine itself; drop its job so no worker picks it up
		if j.Payload.RunID == r.ID {
			_ = c.Delete(context.Background(), "jobs", j.ID)
		}
	}

	if err := e.Execute(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if r, err = st.GetRun(ctx, r.ID); err != nil || r.Status != store.RunDone || r.WaitingOnCardID != nil {
		t.Fatalf("after resume: %+v %v", r, err)
	}
	steps, _ := st.ListSteps(ctx, r.ID)
	last, err := st.ListCards(ctx, steps[len(steps)-1].ID)
	if err != nil || len(last) != 3 {
		t.Fatalf("pile after Check first: %d %v", len(last), err)
	}
	t.Logf("run %s paused, resumed, %d of 4 cards went on", r.ID, len(last))
}

package eurobase_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/eurobase/eurobasetest"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
	"github.com/STGime/laamo/backend/internal/store/storetest"
)

// noSleep records waits instead of sleeping.
type noSleep struct{ waits []time.Duration }

func (n *noSleep) sleep(_ context.Context, d time.Duration) error {
	n.waits = append(n.waits, d)
	return nil
}

func newStore(t *testing.T) (*sebs.Store, *eurobasetest.Fake, *noSleep, storetest.Fixture) {
	t.Helper()
	fake := eurobasetest.New(t)
	ns := &noSleep{}
	c := eb.New(fake.URL(), fake.Key, eb.WithSleep(ns.sleep))
	user := store.NewID()
	w := fake.Put("workspaces", eurobasetest.Row{"id": store.NewID(), "user_id": user, "name": "Bäckerei Krause"})
	h := fake.Put("helpers", eurobasetest.Row{"id": store.NewID(), "workspace_id": w["id"], "user_id": user,
		"name": "Morning Briefing", "status": "active"})
	v := fake.Put("helper_versions", eurobasetest.Row{"id": store.NewID(), "workspace_id": w["id"], "user_id": user,
		"helper_id": h["id"], "version": 1, "stack": eurobasetest.Row{"bricks": []any{}}})
	fx := storetest.Fixture{WorkspaceID: w["id"].(string), UserID: user, HelperID: h["id"].(string),
		VersionID: v["id"].(string), Timezone: "Europe/Berlin"}
	return sebs.New(c), fake, ns, fx
}

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) (store.Store, storetest.Fixture) {
		s, _, _, fx := newStore(t)
		return s, fx
	})
}

func newRun(fx storetest.Fixture) store.Run {
	return store.Run{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: fx.HelperID,
		VersionID: fx.VersionID, TriggerType: "manual"}
}

func TestRateLimitHonoursRetryAfter(t *testing.T) {
	s, fake, ns, fx := newStore(t)
	fake.FailNext(http.MethodPost, "/v1/db/runs", eurobasetest.Fault{Status: 429, RetryAfter: "3"})
	r := newRun(fx)
	if err := s.CreateRun(context.Background(), &r); err != nil {
		t.Fatal(err)
	}
	if len(ns.waits) != 1 || ns.waits[0] != 3*time.Second {
		t.Fatalf("want one 3s wait, got %v", ns.waits)
	}
	if n := len(fake.Rows("runs")); n != 1 {
		t.Fatalf("want 1 run, got %d", n)
	}
}

// The insert went through but the answer was lost: the retry must not create
// a second row or report a conflict.
func TestInsertRetryAfterLostResponse(t *testing.T) {
	s, fake, _, fx := newStore(t)
	fake.FailNext(http.MethodPost, "/v1/db/feed_cards", eurobasetest.Fault{Status: 502, After: true})
	c := store.FeedCard{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, Type: "result", Title: "Today"}
	if err := s.CreateFeedCard(context.Background(), &c); err != nil {
		t.Fatalf("retry after lost response: %v", err)
	}
	if n := len(fake.Rows("feed_cards")); n != 1 {
		t.Fatalf("want 1 feed card, got %d", n)
	}
	if c.State != "open" || c.CreatedAt.IsZero() {
		t.Fatalf("row not read back: %+v", c)
	}
}

// A real conflict on the first try stays a conflict.
func TestOutboundConflictIsNotHidden(t *testing.T) {
	s, _, _, fx := newStore(t)
	ctx := context.Background()
	m := store.OutboundMessage{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, IdempotencyKey: "k1",
		Channel: "discord", Recipient: "#general"}
	if err := s.CreateOutbound(ctx, &m); err != nil {
		t.Fatal(err)
	}
	again := m
	again.ID = ""
	if err := s.CreateOutbound(ctx, &again); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestNotFoundKeepsDetail(t *testing.T) {
	s, _, _, _ := newStore(t)
	_, err := s.GetRun(context.Background(), store.NewID())
	if !errors.Is(err, store.ErrNotFound) || !strings.Contains(err.Error(), "row not found") {
		t.Fatalf("got %v", err)
	}
}

func cardPile(fx storetest.Fixture, runID, stepID string, n, bodySize int) []store.Card {
	cards := make([]store.Card, n)
	for i := range cards {
		cards[i] = store.Card{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: runID, StepID: stepID,
			CardID: store.NewID(), Position: i, Kind: "page",
			Data: map[string]any{"text": strings.Repeat("x", bodySize)}}
	}
	return cards
}

func setupStep(t *testing.T, s *sebs.Store, fx storetest.Fixture) (string, string) {
	t.Helper()
	ctx := context.Background()
	r := newRun(fx)
	if err := s.CreateRun(ctx, &r); err != nil {
		t.Fatal(err)
	}
	st := store.Step{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: r.ID, BrickID: "b2", BrickType: "look.read_page"}
	if err := s.CreateStep(ctx, &st); err != nil {
		t.Fatal(err)
	}
	return r.ID, st.ID
}

func TestInsertCardsChunks(t *testing.T) {
	s, fake, _, fx := newStore(t)
	runID, stepID := setupStep(t, s, fx)
	s.SetChunkLimits(5_000, 4) // ~1.2 KB per card → 3 per chunk by size

	if err := s.InsertCards(context.Background(), cardPile(fx, runID, stepID, 10, 1200)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, r := range fake.Requests {
		if r == "POST /v1/db/rpc/insert_cards" {
			calls++
		}
	}
	if calls != 4 { // 3+3+3+1
		t.Fatalf("want 4 insert_cards calls, got %d", calls)
	}
	got, err := s.ListCards(context.Background(), stepID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 || got[0].Position != 0 || got[9].Position != 9 {
		t.Fatalf("cards after chunked insert: %d", len(got))
	}

	s.SetChunkLimits(1_000_000, 4) // row cap only: 4+4+2
	fake.Requests = nil
	_, step2 := setupStep(t, s, fx)
	if err := s.InsertCards(context.Background(), cardPile(fx, runID, step2, 10, 10)); err != nil {
		t.Fatal(err)
	}
	calls = 0
	for _, r := range fake.Requests {
		if r == "POST /v1/db/rpc/insert_cards" {
			calls++
		}
	}
	if calls != 3 {
		t.Fatalf("want 3 calls with a 4-row cap, got %d", calls)
	}
}

func TestInsertCardsTooLarge(t *testing.T) {
	s, fake, _, fx := newStore(t)
	runID, stepID := setupStep(t, s, fx)
	err := s.InsertCards(context.Background(), cardPile(fx, runID, stepID, 1, eb.MaxBody))
	if !errors.Is(err, store.ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if n := len(fake.Rows("cards")); n != 0 {
		t.Fatalf("nothing should be written, got %d cards", n)
	}
}

// insert_cards ran but the answer was lost: the store checks the first card
// and does not insert the chunk again.
func TestInsertCardsUncertainButApplied(t *testing.T) {
	s, fake, _, fx := newStore(t)
	runID, stepID := setupStep(t, s, fx)
	fake.FailNext(http.MethodPost, "/v1/db/rpc/insert_cards", eurobasetest.Fault{Status: 504, After: true})
	if err := s.InsertCards(context.Background(), cardPile(fx, runID, stepID, 3, 10)); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Rows("cards")); n != 3 {
		t.Fatalf("want 3 cards, got %d", n)
	}
}

// insert_cards never ran (502 before applying): the store retries the chunk.
func TestInsertCardsUncertainNotApplied(t *testing.T) {
	s, fake, _, fx := newStore(t)
	runID, stepID := setupStep(t, s, fx)
	fake.FailNext(http.MethodPost, "/v1/db/rpc/insert_cards", eurobasetest.Fault{Status: 502})
	if err := s.InsertCards(context.Background(), cardPile(fx, runID, stepID, 3, 10)); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Rows("cards")); n != 3 {
		t.Fatalf("want 3 cards, got %d", n)
	}
}

func TestListRunsPagesPastOneThousand(t *testing.T) {
	s, fake, _, fx := newStore(t)
	for range 1005 {
		fake.Put("runs", eurobasetest.Row{"id": store.NewID(), "workspace_id": fx.WorkspaceID, "user_id": fx.UserID,
			"helper_id": fx.HelperID, "version_id": fx.VersionID, "trigger_type": "schedule"})
	}
	runs, err := s.ListRuns(context.Background(), store.RunFilter{HelperID: fx.HelperID})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1005 {
		t.Fatalf("want 1005 runs over two pages, got %d", len(runs))
	}
	seen := map[string]bool{}
	for _, r := range runs {
		if seen[r.ID] {
			t.Fatalf("run %s returned twice", r.ID)
		}
		seen[r.ID] = true
	}
}

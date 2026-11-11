// Package storetest is a conformance suite every store.Store implementation
// must pass. Each implementation's test calls Run with a constructor.
package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/store"
)

// Fixture holds rows the constructor seeded (the API creates these in real
// life; the Store interface only reads them).
type Fixture struct {
	WorkspaceID string
	UserID      string
	HelperID    string
	VersionID   string
	Timezone    string
}

// New returns a fresh, empty store seeded with one workspace, helper and
// helper version. Creates must get increasing created_at values, so "newest
// first" ordering is stable.
type New func(t *testing.T) (store.Store, Fixture)

func Run(t *testing.T, newStore New) {
	t.Run("ReadSeeded", func(t *testing.T) { testReadSeeded(t, newStore) })
	t.Run("Runs", func(t *testing.T) { testRuns(t, newStore) })
	t.Run("ListRuns", func(t *testing.T) { testListRuns(t, newStore) })
	t.Run("Steps", func(t *testing.T) { testSteps(t, newStore) })
	t.Run("Cards", func(t *testing.T) { testCards(t, newStore) })
	t.Run("FeedCards", func(t *testing.T) { testFeed(t, newStore) })
	t.Run("Lists", func(t *testing.T) { testLists(t, newStore) })
	t.Run("Usage", func(t *testing.T) { testUsage(t, newStore) })
	t.Run("Outbound", func(t *testing.T) { testOutbound(t, newStore) })
	t.Run("Workspaces", func(t *testing.T) { testWorkspaces(t, newStore) })
	t.Run("Brains", func(t *testing.T) { testBrains(t, newStore) })
	t.Run("Triggers", func(t *testing.T) { testTriggers(t, newStore) })
	t.Run("Places", func(t *testing.T) { testPlaces(t, newStore) })
}

func testReadSeeded(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()

	w, err := s.GetWorkspace(ctx, fx.WorkspaceID)
	must(t, err)
	eq(t, "workspace user", w.UserID, fx.UserID)
	eq(t, "workspace timezone", w.Timezone, fx.Timezone)

	h, err := s.GetHelper(ctx, fx.HelperID)
	must(t, err)
	eq(t, "helper workspace", h.WorkspaceID, fx.WorkspaceID)

	v, err := s.GetHelperVersion(ctx, fx.VersionID)
	must(t, err)
	eq(t, "version helper", v.HelperID, fx.HelperID)
	if !json.Valid(v.Stack) || len(v.Stack) == 0 {
		t.Fatalf("version stack is not JSON: %q", v.Stack)
	}

	isErr(t, "unknown workspace", get(s.GetWorkspace(ctx, store.NewID())), store.ErrNotFound)
	isErr(t, "unknown helper", get(s.GetHelper(ctx, store.NewID())), store.ErrNotFound)
	isErr(t, "unknown version", get(s.GetHelperVersion(ctx, store.NewID())), store.ErrNotFound)
}

func newRun(fx Fixture) store.Run {
	return store.Run{
		WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: fx.HelperID, VersionID: fx.VersionID,
		TriggerType: "manual", TriggerData: map[string]any{"answer": "bakeries in Leipzig"},
	}
}

func testRuns(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()

	r := newRun(fx)
	must(t, s.CreateRun(ctx, &r))
	if r.ID == "" || r.CreatedAt.IsZero() {
		t.Fatalf("create should fill id and created_at: %+v", r)
	}
	eq(t, "default status", r.Status, store.RunQueued)

	got, err := s.GetRun(ctx, r.ID)
	must(t, err)
	eq(t, "trigger data", got.TriggerData["answer"], "bakeries in Leipzig")
	eq(t, "test mode", got.TestMode, false)

	// set, then clear a nullable column; other fields stay
	cardID := store.NewID()
	started := time.Now().UTC().Truncate(time.Millisecond)
	got, err = s.UpdateRun(ctx, r.ID, store.RunPatch{
		Status:          new(store.RunWaiting),
		NextBrickIndex:  new(3),
		WaitingOnCardID: store.Set(cardID),
		StartedAt:       &started,
		CostEUR:         new(store.Num(0.0125)),
	})
	must(t, err)
	eq(t, "status", got.Status, store.RunWaiting)
	eq(t, "next brick", got.NextBrickIndex, 3)
	if got.WaitingOnCardID == nil || *got.WaitingOnCardID != cardID {
		t.Fatalf("waiting_on_card_id not set: %v", got.WaitingOnCardID)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(started) {
		t.Fatalf("started_at = %v, want %v", got.StartedAt, started)
	}
	eq(t, "cost", got.CostEUR, store.Num(0.0125))

	got, err = s.UpdateRun(ctx, r.ID, store.RunPatch{WaitingOnCardID: store.Null[string](), Status: new(store.RunRunning)})
	must(t, err)
	if got.WaitingOnCardID != nil {
		t.Fatalf("waiting_on_card_id should be cleared, got %v", *got.WaitingOnCardID)
	}
	eq(t, "next brick kept", got.NextBrickIndex, 3)
	eq(t, "trigger data kept", got.TriggerData["answer"], "bakeries in Leipzig")

	got, err = s.GetRun(ctx, r.ID)
	must(t, err)
	eq(t, "persisted status", got.Status, store.RunRunning)

	isErr(t, "update unknown run", get(s.UpdateRun(ctx, store.NewID(), store.RunPatch{Status: new(store.RunDone)})), store.ErrNotFound)

	// returned rows are copies
	got.TriggerData["answer"] = "changed"
	again, err := s.GetRun(ctx, r.ID)
	must(t, err)
	eq(t, "copy", again.TriggerData["answer"], "bakeries in Leipzig")

	dupe := newRun(fx)
	dupe.ID = r.ID
	isErr(t, "duplicate run id", s.CreateRun(ctx, &dupe), store.ErrConflict)
}

func testListRuns(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()

	var ids []string
	for i := range 3 {
		r := newRun(fx)
		if i == 1 {
			r.Status = store.RunWaiting
		}
		must(t, s.CreateRun(ctx, &r))
		ids = append(ids, r.ID)
	}

	all, err := s.ListRuns(ctx, store.RunFilter{HelperID: fx.HelperID})
	must(t, err)
	if len(all) != 3 || all[0].ID != ids[2] || all[2].ID != ids[0] {
		t.Fatalf("want 3 runs newest first, got %d: %v", len(all), runIDs(all))
	}
	top, err := s.ListRuns(ctx, store.RunFilter{WorkspaceID: fx.WorkspaceID, Limit: 2})
	must(t, err)
	eq(t, "limit", len(top), 2)

	waiting, err := s.ListRuns(ctx, store.RunFilter{Status: store.RunWaiting})
	must(t, err)
	if len(waiting) != 1 || waiting[0].ID != ids[1] {
		t.Fatalf("status filter: %v", runIDs(waiting))
	}
	none, err := s.ListRuns(ctx, store.RunFilter{HelperID: store.NewID()})
	must(t, err)
	eq(t, "no runs", len(none), 0)
}

func testSteps(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	r := newRun(fx)
	must(t, s.CreateRun(ctx, &r))

	mk := func(brick string, pos, attempt int) store.Step {
		return store.Step{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: r.ID,
			BrickID: brick, BrickType: "look.search", Position: pos, Attempt: attempt}
	}
	b2 := mk("b2", 1, 0)
	must(t, s.CreateStep(ctx, &b2))
	eq(t, "default attempt", b2.Attempt, 1)
	eq(t, "default status", b2.Status, "running")
	if b2.StartedAt.IsZero() {
		t.Fatal("started_at should default to now")
	}
	b1 := mk("b1", 0, 1)
	must(t, s.CreateStep(ctx, &b1))
	b2retry := mk("b2", 1, 2)
	must(t, s.CreateStep(ctx, &b2retry))

	same := mk("b2", 1, 2)
	isErr(t, "same run+brick+attempt", s.CreateStep(ctx, &same), store.ErrConflict)

	steps, err := s.ListSteps(ctx, r.ID)
	must(t, err)
	if len(steps) != 3 || steps[0].ID != b1.ID || steps[1].ID != b2.ID || steps[2].ID != b2retry.ID {
		t.Fatalf("steps by position, attempt: %+v", steps)
	}

	done := time.Now().UTC().Truncate(time.Millisecond)
	st, err := s.UpdateStep(ctx, b1.ID, store.StepPatch{
		Status: new("done"), CardsOut: new(12), FinishedAt: &done,
		Log: []map[string]any{{"msg": "searched", "results": float64(12)}},
	})
	must(t, err)
	eq(t, "step status", st.Status, "done")
	eq(t, "cards out", st.CardsOut, 12)
	eq(t, "brick kept", st.BrickID, "b1")
	if len(st.Log) != 1 || st.Log[0]["msg"] != "searched" {
		t.Fatalf("log: %v", st.Log)
	}
	isErr(t, "update unknown step", get(s.UpdateStep(ctx, store.NewID(), store.StepPatch{Status: new("done")})), store.ErrNotFound)
}

func testCards(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	r := newRun(fx)
	must(t, s.CreateRun(ctx, &r))
	st := store.Step{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: r.ID, BrickID: "b2", BrickType: "look.search", Position: 1}
	must(t, s.CreateStep(ctx, &st))

	mk := func(pos int, title string) store.Card {
		return store.Card{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: r.ID, StepID: st.ID,
			CardID: store.NewID(), Position: pos, Kind: "web_result",
			Data:  map[string]any{"title": title},
			Trust: map[string]string{"title": "untrusted"}}
	}
	pile := []store.Card{mk(2, "c"), mk(0, "a"), mk(1, "b")}
	pile[0].SourceURL = new("https://example.com/c")
	pile[0].Score = new(store.Num(0.75))
	must(t, s.InsertCards(ctx, pile))
	for i, c := range pile {
		if c.ID == "" {
			t.Fatalf("card %d: id not filled", i)
		}
	}
	must(t, s.InsertCards(ctx, nil))

	got, err := s.ListCards(ctx, st.ID)
	must(t, err)
	if len(got) != 3 {
		t.Fatalf("want 3 cards, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		eq[any](t, "card order", got[i].Data["title"], want)
	}
	eq(t, "trust", got[2].Trust["title"], "untrusted")
	if got[2].SourceURL == nil || *got[2].SourceURL != "https://example.com/c" {
		t.Fatalf("source_url: %v", got[2].SourceURL)
	}
	if got[2].Score == nil || *got[2].Score != 0.75 {
		t.Fatalf("score: %v", got[2].Score)
	}

	// all-or-nothing: a duplicate card_id in the same step rejects the pile
	dupe := mk(3, "d")
	dupe.CardID = pile[0].CardID
	isErr(t, "duplicate card in step", s.InsertCards(ctx, []store.Card{mk(4, "e"), dupe}), store.ErrConflict)
	got, err = s.ListCards(ctx, st.ID)
	must(t, err)
	eq(t, "pile unchanged", len(got), 3)

	none, err := s.ListCards(ctx, store.NewID())
	must(t, err)
	eq(t, "no cards", len(none), 0)
}

func testFeed(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	r := newRun(fx)
	must(t, s.CreateRun(ctx, &r))

	c := store.FeedCard{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: &fx.HelperID, RunID: &r.ID,
		Type: "approval", Title: "Send 3 emails?", Payload: map[string]any{"cards": []any{"x"}}}
	must(t, s.CreateFeedCard(ctx, &c))
	eq(t, "default state", c.State, "open")

	when := time.Now().UTC().Truncate(time.Millisecond)
	got, err := s.UpdateFeedCard(ctx, c.ID, store.FeedCardPatch{
		State: new("edited"), Decision: map[string]any{"edits": map[string]any{"subject": "Hi"}},
		DecidedBy: &fx.UserID, DecidedAt: &when,
	})
	must(t, err)
	eq(t, "state", got.State, "edited")
	eq(t, "title kept", got.Title, "Send 3 emails?")

	got, err = s.GetFeedCard(ctx, c.ID)
	must(t, err)
	edits, _ := got.Decision["edits"].(map[string]any)
	eq(t, "decision", edits["subject"], "Hi")
	if got.DecidedAt == nil || !got.DecidedAt.Equal(when) {
		t.Fatalf("decided_at = %v", got.DecidedAt)
	}
	isErr(t, "unknown feed card", get(s.GetFeedCard(ctx, store.NewID())), store.ErrNotFound)

	// An Ask me card uses its step's id, so a retried pause can't make two.
	st := store.Step{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: r.ID, BrickID: "b3", BrickType: "ask.check",
		Position: 2, Status: "waiting"}
	must(t, s.CreateStep(ctx, &st))
	eq(t, "step status", st.Status, "waiting")
	ask := store.FeedCard{ID: st.ID, WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: &r.ID, StepID: &st.ID,
		Type: "approval", Title: "Check 3", ExpiresAt: new(when.Add(48 * time.Hour))}
	must(t, s.CreateFeedCard(ctx, &ask))
	eq(t, "given id kept", ask.ID, st.ID)
	again := ask
	isErr(t, "same feed card id", s.CreateFeedCard(ctx, &again), store.ErrConflict)
	_, err = s.UpdateRun(ctx, r.ID, store.RunPatch{Status: new(store.RunWaiting), WaitingOnCardID: store.Set(ask.ID)})
	must(t, err)
	got2, err := s.UpdateRun(ctx, r.ID, store.RunPatch{Status: new(store.RunRunning), WaitingOnCardID: store.Null[string]()})
	must(t, err)
	if got2.WaitingOnCardID != nil {
		t.Fatalf("waiting_on_card_id not cleared: %v", *got2.WaitingOnCardID)
	}
}

func testLists(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()

	l := store.List{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, Name: "Leads", DedupeField: new("website")}
	must(t, s.CreateList(ctx, &l))
	other := store.List{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, Name: "Leads"}
	isErr(t, "same list name", s.CreateList(ctx, &other), store.ErrConflict)

	got, err := s.GetListByName(ctx, fx.WorkspaceID, "Leads")
	must(t, err)
	eq(t, "list id", got.ID, l.ID)
	isErr(t, "unknown list", get(s.GetListByName(ctx, fx.WorkspaceID, "Nope")), store.ErrNotFound)
	isErr(t, "other workspace", get(s.GetListByName(ctx, store.NewID(), "Leads")), store.ErrNotFound)

	row := func(site string) store.ListRow {
		return store.ListRow{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, ListID: l.ID,
			Data: map[string]any{"website": site}, DedupeKey: new(site)}
	}
	r1 := row("a.de")
	must(t, s.AddListRow(ctx, &r1))
	r2 := row("b.de")
	must(t, s.AddListRow(ctx, &r2))
	again := row("a.de")
	isErr(t, "dedupe key", s.AddListRow(ctx, &again), store.ErrConflict)
	free := store.ListRow{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, ListID: l.ID, Data: map[string]any{"note": "x"}}
	must(t, s.AddListRow(ctx, &free))
	// no dedupe key: duplicates allowed. (A fresh row: a copy of a stored one
	// would carry its created_at, and the database keeps explicit values.)
	free2 := store.ListRow{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, ListID: l.ID, Data: map[string]any{"note": "x"}}
	must(t, s.AddListRow(ctx, &free2))

	rows, err := s.ListRows(ctx, l.ID, 0)
	must(t, err)
	if len(rows) != 4 || rows[0].ID != free2.ID || rows[3].ID != r1.ID {
		t.Fatalf("rows newest first: %d", len(rows))
	}
	rows, err = s.ListRows(ctx, l.ID, 2)
	must(t, err)
	eq(t, "rows limit", len(rows), 2)
}

func testUsage(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	u := store.UsageEntry{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, Kind: "llm",
		BrainMode: new("included"), Model: new("mistral-small"),
		Units: 1200, BrainCredits: 1.2, EstCostEUR: 0.00042}
	must(t, s.AddUsage(ctx, &u))
	if u.ID == "" || u.CreatedAt.IsZero() {
		t.Fatalf("usage id/created_at not filled: %+v", u)
	}
}

func testOutbound(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	r := newRun(fx)
	must(t, s.CreateRun(ctx, &r))

	key := r.ID + ":b5:" + store.NewID()
	m := store.OutboundMessage{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, RunID: &r.ID,
		IdempotencyKey: key, Channel: "email", Lane: new(1), Recipient: "owner@example.com",
		Subject: new("Your morning briefing")}
	must(t, s.CreateOutbound(ctx, &m))
	eq(t, "default status", m.Status, "pending")

	retry := m
	retry.ID = ""
	isErr(t, "same idempotency key", s.CreateOutbound(ctx, &retry), store.ErrConflict)

	got, err := s.GetOutboundByKey(ctx, key)
	must(t, err)
	eq(t, "by key", got.ID, m.ID)
	isErr(t, "unknown key", get(s.GetOutboundByKey(ctx, "nope")), store.ErrNotFound)

	sent := time.Now().UTC().Truncate(time.Millisecond)
	got, err = s.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: new("sent"),
		ProviderMessageID: new("tem-123"), SentAt: &sent})
	must(t, err)
	eq(t, "sent", got.Status, "sent")
	eq(t, "recipient kept", got.Recipient, "owner@example.com")
}

func testWorkspaces(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	owner := store.NewID()

	w, err := s.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: owner, Name: "  Bäckerei Krause ", Timezone: "Europe/Vienna", Locale: "de"})
	must(t, err)
	eq(t, "name trimmed", w.Name, "Bäckerei Krause")
	eq(t, "owner", w.UserID, owner)
	eq(t, "timezone", w.Timezone, "Europe/Vienna")
	eq(t, "plan", w.Plan, "free")

	d, err := s.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: owner, Name: "Second"})
	must(t, err)
	eq(t, "default timezone", d.Timezone, "Europe/Berlin")
	eq(t, "default locale", d.Locale, "en")

	m, err := s.Membership(ctx, w.ID, owner)
	must(t, err)
	eq(t, "owner role", m.Role, store.RoleOwner)
	isErr(t, "not a member", get(s.Membership(ctx, w.ID, store.NewID())), store.ErrNotFound)
	isErr(t, "seeded workspace, no member", get(s.Membership(ctx, fx.WorkspaceID, owner)), store.ErrNotFound)

	ms, err := s.Memberships(ctx, owner)
	must(t, err)
	if len(ms) != 2 || ms[0].WorkspaceID != w.ID || ms[1].WorkspaceID != d.ID {
		t.Fatalf("memberships oldest first: %+v", ms)
	}
	none, err := s.Memberships(ctx, store.NewID())
	must(t, err)
	eq(t, "no memberships", len(none), 0)

	got, err := s.GetWorkspaces(ctx, []string{w.ID, store.NewID(), d.ID})
	must(t, err)
	eq(t, "get workspaces skips unknown ids", len(got), 2)

	for name, nw := range map[string]store.NewWorkspace{
		"empty name":   {OwnerID: owner, Name: "  "},
		"bad timezone": {OwnerID: owner, Name: "x", Timezone: "Mars/Olympus"},
		"bad locale":   {OwnerID: owner, Name: "x", Locale: "fr"},
	} {
		isErr(t, name, get(s.CreateWorkspace(ctx, nw)), store.ErrInvalid)
	}
	for range 3 {
		_, err := s.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: owner, Name: "filler"})
		must(t, err)
	}
	isErr(t, "sixth workspace", get(s.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: owner, Name: "sixth"})), store.ErrInvalid)

	u, err := s.UpdateWorkspace(ctx, w.ID, store.WorkspacePatch{Name: new("Krause & Söhne"),
		Profile: map[string]any{"city": "Wien", "business": "bakery"}})
	must(t, err)
	eq(t, "renamed", u.Name, "Krause & Söhne")
	eq(t, "profile", u.Profile["city"], any("Wien"))
	eq(t, "timezone kept", u.Timezone, "Europe/Vienna")
	isErr(t, "update unknown", get(s.UpdateWorkspace(ctx, store.NewID(), store.WorkspacePatch{Name: new("x")})), store.ErrNotFound)
}

func testBrains(t *testing.T, newStore New) {
	s, _ := newStore(t)
	ctx := context.Background()
	owner := store.NewID()
	w, err := s.CreateWorkspace(ctx, store.NewWorkspace{OwnerID: owner, Name: "Brainy"})
	must(t, err)

	b, err := s.GetBrain(ctx, w.ID)
	must(t, err)
	eq(t, "mode", b.Mode, "included")
	eq(t, "allowance", b.AllowanceCredits, store.Num(500))
	eq(t, "credits left", b.CreditsLeft(time.Now()), 500.0)

	b, err = s.AddBrainUsage(ctx, w.ID, 120.5)
	must(t, err)
	eq(t, "used", b.UsedCreditsThisPeriod, store.Num(120.5))
	eq(t, "status", b.Status, "ok")
	b, err = s.AddBrainUsage(ctx, w.ID, 400)
	must(t, err)
	eq(t, "allowance used", b.Status, "allowance_used")
	if left := b.CreditsLeft(time.Now()); left > -20 || left < -21 {
		t.Fatalf("credits left: %v", left)
	}

	isErr(t, "no brain", get(s.GetBrain(ctx, store.NewID())), store.ErrNotFound)
	isErr(t, "usage without brain", get(s.AddBrainUsage(ctx, store.NewID(), 1)), store.ErrNotFound)
	isErr(t, "negative credits", get(s.AddBrainUsage(ctx, w.ID, -1)), store.ErrInvalid)

	// A brain from last month has its full allowance this month.
	last := store.Brain{AllowanceCredits: 500, UsedCreditsThisPeriod: 499, PeriodStart: time.Now().AddDate(0, -1, 0)}
	eq(t, "new month", last.CreditsLeft(time.Now()), 500.0)
}

func testTriggers(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	next := time.Date(2026, 11, 11, 6, 30, 0, 0, time.UTC)
	tr := store.Trigger{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: fx.HelperID, Type: store.TriggerSchedule,
		Config: map[string]any{"freq": "weekdays", "time": "07:30"}, NextRunAt: &next}
	must(t, s.CreateTrigger(ctx, &tr))
	addr := "briefing.x7k@in.laamo.app"
	mail := store.Trigger{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: fx.HelperID, Type: store.TriggerEmail, InboundAddress: &addr}
	must(t, s.CreateTrigger(ctx, &mail))
	again := store.Trigger{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, HelperID: fx.HelperID, Type: store.TriggerEmail, InboundAddress: &addr}
	isErr(t, "same inbound address", s.CreateTrigger(ctx, &again), store.ErrConflict)

	got, err := s.HelperTriggers(ctx, fx.HelperID)
	must(t, err)
	if len(got) != 2 || got[0].ID != tr.ID || got[0].Config["time"] != "07:30" || got[0].NextRunAt == nil || !got[0].NextRunAt.Equal(next) {
		t.Fatalf("triggers: %+v", got)
	}

	later := next.Add(24 * time.Hour)
	u, err := s.UpdateTrigger(ctx, tr.ID, store.TriggerPatch{NextRunAt: store.Set(later), Config: map[string]any{"freq": "daily", "time": "07:30"}})
	must(t, err)
	if !u.NextRunAt.Equal(later) || u.Config["freq"] != "daily" {
		t.Fatalf("update: %+v", u)
	}
	u, err = s.UpdateTrigger(ctx, tr.ID, store.TriggerPatch{NextRunAt: store.Null[time.Time]()})
	must(t, err)
	if u.NextRunAt != nil {
		t.Fatal("next_run_at should be cleared (helper paused)")
	}

	must(t, s.DeleteTrigger(ctx, mail.ID))
	must(t, s.DeleteTrigger(ctx, mail.ID)) // already gone: fine
	got, err = s.HelperTriggers(ctx, fx.HelperID)
	must(t, err)
	eq(t, "after delete", len(got), 1)
}

func testPlaces(t *testing.T, newStore New) {
	s, fx := newStore(t)
	ctx := context.Background()
	p := store.Place{WorkspaceID: fx.WorkspaceID, UserID: fx.UserID, Type: "discord", Label: "#news",
		SecretCiphertext: new("AQJ2MQ…base64"), SecretLast4: new("…x9Zq")}
	must(t, s.CreatePlace(ctx, &p))
	got, err := s.GetPlace(ctx, p.ID)
	must(t, err)
	if got.Type != "discord" || got.Label != "#news" || got.SecretCiphertext == nil || *got.SecretCiphertext != "AQJ2MQ…base64" {
		t.Fatalf("place: %+v", got)
	}
	isErr(t, "unknown place", get(s.GetPlace(ctx, store.NewID())), store.ErrNotFound)
}

// ---- assertions ----

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func isErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: got error %v, want %v", what, err, target)
	}
}

func get[T any](_ T, err error) error { return err }

func runIDs(rs []store.Run) []string {
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	return ids
}

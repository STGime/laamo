package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// ---- fixtures ----

type sb = catalog.StackBrick
type cfg = map[string]any

// calls counts brick calls by type.
type calls struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *calls) inc(t string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[t]++
	return c.n[t]
}

func (c *calls) get(t string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[t]
}

type env struct {
	t      *testing.T
	store  *memory.Store
	eng    *engine.Engine
	blobs  *engine.MemoryBlobs
	calls  *calls
	ws     store.Workspace
	helper store.Helper
}

// fakeBricks behave like real ones, minus the outside world.
func fakeBricks(c *calls, searchResults int) map[string]engine.Brick {
	pass := func(t string) engine.Brick {
		return engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
			c.inc(t)
			return in, nil
		})
	}
	return map[string]engine.Brick{
		"when.every": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
			c.inc("when.every")
			return nil, nil
		}),
		"when.tap": pass("when.tap"),
		"look.search": engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, cf engine.Config) ([]engine.Card, error) {
			c.inc("look.search")
			var out []engine.Card
			for i := range searchResults {
				var card engine.Card
				card.Kind = "web_result"
				card.Set("title", fmt.Sprintf("%s #%d", cf.String("query"), i), engine.TrustUntrusted)
				card.Set("url", fmt.Sprintf("https://example.com/%d", i), engine.TrustUntrusted)
				card.Source = card.Text("url")
				out = append(out, card)
			}
			return out, rc.AddCost(0.001)
		}),
		"think.keep": engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, cf engine.Config) ([]engine.Card, error) {
			c.inc("think.keep")
			keep := in[:min(cf.Int("max"), len(in))]
			for i := range keep {
				score := 9.0 - float64(i)
				keep[i].Score, keep[i].Reason = &score, "relevant"
				keep[i].Set("score", score, engine.TrustExtracted)
			}
			return keep, nil
		}),
		"think.summarize": engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
			c.inc("think.summarize")
			var card engine.Card
			card.Kind = "summary"
			city, _ := rc.Profile("city")
			card.Set("summary", fmt.Sprintf("%d news for %s", len(in), city), engine.TrustExtracted)
			return []engine.Card{card}, rc.AddCost(0.002)
		}),
		"do.chat": pass("do.chat"),
	}
}

func newEnv(t *testing.T, stack []sb, opts ...func(*catalog.Stack)) *env {
	t.Helper()
	st := memory.New()
	user := store.NewID()
	ws := st.PutWorkspace(store.Workspace{UserID: user, Name: "Bäckerei", Locale: "en", Profile: map[string]any{"city": "Leipzig"}})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: user, Name: "Morning Briefing", Status: "active"})
	s := catalog.Stack{Schema: 1, Bricks: stack}
	for _, o := range opts {
		o(&s)
	}
	raw, _ := json.Marshal(s)
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: user, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	h = st.PutHelper(h)

	c := &calls{}
	blobs := &engine.MemoryBlobs{}
	e := &engine.Engine{Store: st, Catalog: catalog.Default(), Bricks: fakeBricks(c, 5), Blobs: blobs,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return &env{t: t, store: st, eng: e, blobs: blobs, calls: c, ws: ws, helper: h}
}

func (e *env) start(trigger string, testMode bool) store.Run {
	e.t.Helper()
	r, err := e.eng.StartRun(context.Background(), engine.StartRequest{HelperID: e.helper.ID, TriggerType: trigger, TestMode: testMode})
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func (e *env) run(r store.Run) store.Run {
	e.t.Helper()
	if err := e.eng.Execute(context.Background(), r.ID); err != nil {
		e.t.Fatalf("execute: %v", err)
	}
	return e.get(r.ID)
}

func (e *env) get(id string) store.Run {
	e.t.Helper()
	r, err := e.store.GetRun(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func (e *env) steps(runID string) []store.Step {
	e.t.Helper()
	s, err := e.store.ListSteps(context.Background(), runID)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) cards(stepID string) []store.Card {
	e.t.Helper()
	c, err := e.store.ListCards(context.Background(), stepID)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// feed returns the Feed cards of a run.
func (e *env) feed(runID string) []store.FeedCard {
	var out []store.FeedCard
	for _, c := range e.store.FeedCards() {
		if c.RunID != nil && *c.RunID == runID {
			out = append(out, c)
		}
	}
	return out
}

func morningBriefing() []sb {
	return []sb{
		{ID: "b1", Type: "when.every", Config: cfg{"freq": "weekdays", "time": "07:30"}},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakery news"}},
		{ID: "b3", Type: "think.keep", Config: cfg{"criteria": "matter to {my business}", "max": 3}},
		{ID: "b4", Type: "think.summarize"},
		{ID: "b5", Type: "do.chat", Config: cfg{"place": "p1"}},
	}
}

// ---- tests ----

func TestRunToTheEnd(t *testing.T) {
	e := newEnv(t, morningBriefing())
	r := e.run(e.start("manual", false))

	if r.Status != store.RunDone || r.NextBrickIndex != 5 || r.StartedAt == nil || r.FinishedAt == nil {
		t.Fatalf("run: %+v", r)
	}
	if float64(r.CostEUR) < 0.0029 || float64(r.CostEUR) > 0.0031 {
		t.Fatalf("cost = %v, want 0.003", r.CostEUR)
	}
	steps := e.steps(r.ID)
	if len(steps) != 5 {
		t.Fatalf("want 5 steps, got %d", len(steps))
	}
	wantIO := [][2]int{{0, 0}, {0, 5}, {5, 3}, {3, 1}, {1, 1}}
	for i, s := range steps {
		if s.Status != "done" || s.Position != i || s.CardsIn != wantIO[i][0] || s.CardsOut != wantIO[i][1] || s.FinishedAt == nil {
			t.Fatalf("step %d: %+v", i, s)
		}
	}

	// Keep the good ones keeps the cards' identity.
	search, keep := e.cards(steps[1].ID), e.cards(steps[2].ID)
	if len(search) != 5 || len(keep) != 3 {
		t.Fatalf("piles: %d, %d", len(search), len(keep))
	}
	for i := range keep {
		if keep[i].CardID != search[i].CardID || keep[i].Score == nil || keep[i].Trust["title"] != engine.TrustUntrusted {
			t.Fatalf("kept card %d lost identity or fields: %+v", i, keep[i])
		}
	}
	if search[0].SourceURL == nil || *search[0].SourceURL != "https://example.com/0" {
		t.Fatalf("source: %v", search[0].SourceURL)
	}

	feed := e.feed(r.ID)
	if len(feed) != 1 || feed[0].Type != "result" || feed[0].Body == nil || *feed[0].Body != "3 news for Leipzig" {
		t.Fatalf("feed: %+v", feed)
	}
	if feed[0].Title != "Done" || feed[0].StepID == nil || feed[0].Payload["cards"] == nil {
		t.Fatalf("result card: %+v", feed[0])
	}

	// Executing a finished run again does nothing.
	e.run(r)
	if e.calls.get("look.search") != 1 {
		t.Fatal("finished run ran again")
	}
}

func TestRetryResumesAtTheFailedBrick(t *testing.T) {
	e := newEnv(t, morningBriefing())
	fails := 1
	e.eng.Bricks["think.summarize"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		if e.calls.inc("think.summarize") <= fails {
			return nil, errors.New("model timeout")
		}
		if len(in) != 3 || in[0].Text("title") == "" || in[0].Score == nil {
			return nil, fmt.Errorf("resumed with a bad pile: %+v", in)
		}
		var c engine.Card
		c.Set("summary", "ok", engine.TrustExtracted)
		return []engine.Card{c}, nil
	})
	r := e.start("manual", false)

	if err := e.eng.Execute(context.Background(), r.ID); err == nil {
		t.Fatal("a transient brick error must be returned so the queue retries")
	}
	if got := e.get(r.ID); got.Status != store.RunRunning || got.NextBrickIndex != 3 {
		t.Fatalf("after the failure: %s at %d", got.Status, got.NextBrickIndex)
	}

	r = e.run(r)
	if r.Status != store.RunDone {
		t.Fatalf("status %s (%v)", r.Status, r.Error)
	}
	if e.calls.get("look.search") != 1 || e.calls.get("think.keep") != 1 {
		t.Fatal("earlier bricks ran again")
	}
	var attempts []int
	for _, s := range e.steps(r.ID) {
		if s.BrickID == "b4" {
			attempts = append(attempts, s.Attempt)
		}
	}
	if !slices.Equal(attempts, []int{1, 2}) {
		t.Fatalf("b4 attempts: %v", attempts)
	}
}

func TestPermanentErrorEndsTheRun(t *testing.T) {
	e := newEnv(t, morningBriefing())
	e.eng.Bricks["think.keep"] = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
		return nil, queue.Permanent(errors.New("the criteria can't be understood"))
	})
	r := e.run(e.start("manual", false))
	if r.Status != store.RunFailed || r.Error == nil {
		t.Fatalf("run: %s %v", r.Status, r.Error)
	}
	feed := e.feed(r.ID)
	if len(feed) != 1 || feed[0].Type != "problem" || !strings.Contains(*feed[0].Body, "Keep the good ones") ||
		!strings.Contains(*feed[0].Body, "can't be understood") {
		t.Fatalf("problem card: %+v", feed)
	}
	if e.calls.get("think.summarize") != 0 {
		t.Fatal("bricks after the failure ran")
	}
}

func TestShapeIsEnforced(t *testing.T) {
	bad := map[string]struct {
		brick string
		fn    func(in []engine.Card) []engine.Card
	}{
		"filter adds a new card": {"think.keep", func(in []engine.Card) []engine.Card { return append(in, engine.Card{Kind: "x"}) }},
		"filter repeats a card":  {"think.keep", func(in []engine.Card) []engine.Card { return append(in[:1], in[0]) }},
		"combine returns two":    {"think.summarize", func([]engine.Card) []engine.Card { return []engine.Card{{}, {}} }},
		"act drops a card":       {"do.chat", func(in []engine.Card) []engine.Card { return nil }},
	}
	for name, tc := range bad {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, morningBriefing())
			e.eng.Bricks[tc.brick] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
				return tc.fn(in), nil
			})
			r := e.run(e.start("manual", false))
			if r.Status != store.RunFailed || r.Error == nil || !strings.Contains(*r.Error, "wrong kind of pile") {
				t.Fatalf("run: %s %v", r.Status, r.Error)
			}
		})
	}
}

func TestSafetyStop(t *testing.T) {
	t.Run("too many cards", func(t *testing.T) {
		e := newEnv(t, morningBriefing())
		e.eng.Bricks = fakeBricks(e.calls, 60)
		r := e.run(e.start("manual", false))
		if r.Status != store.RunStopped || e.calls.get("think.keep") != 0 {
			t.Fatalf("run: %s, keep calls %d", r.Status, e.calls.get("think.keep"))
		}
		if f := e.feed(r.ID); len(f) != 1 || f[0].Type != "problem" || !strings.Contains(*f[0].Body, "60 cards") {
			t.Fatalf("feed: %+v", f)
		}
	})
	t.Run("Safety stop brick is stricter", func(t *testing.T) {
		stack := morningBriefing()
		stack = slices.Insert(stack, 2, sb{ID: "b9", Type: "ask.stop", Config: cfg{"max_cards": 4}})
		e := newEnv(t, stack)
		e.eng.Bricks["ask.stop"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
			return in, nil
		})
		r := e.run(e.start("manual", false))
		if r.Status != store.RunStopped || r.NextBrickIndex != 2 {
			t.Fatalf("run: %s at %d", r.Status, r.NextBrickIndex)
		}
	})
	t.Run("stack limits", func(t *testing.T) {
		e := newEnv(t, morningBriefing(), func(s *catalog.Stack) { s.Limits = &catalog.Limits{MaxCards: new(2)} })
		if r := e.run(e.start("manual", false)); r.Status != store.RunStopped {
			t.Fatalf("run: %s", r.Status)
		}
	})
	t.Run("cost limit", func(t *testing.T) {
		e := newEnv(t, morningBriefing(), func(s *catalog.Stack) { s.Limits = &catalog.Limits{MaxCostEUR: new(0.0015)} })
		r := e.run(e.start("manual", false))
		// search costs 0.001, summarize 0.002 → over the limit inside summarize.
		if r.Status != store.RunStopped || e.calls.get("do.chat") != 0 {
			t.Fatalf("run: %s, chat %d", r.Status, e.calls.get("do.chat"))
		}
		if f := e.feed(r.ID); !strings.Contains(*f[0].Body, "€0.00") && !strings.Contains(*f[0].Body, "cost") {
			t.Fatalf("feed: %s", *f[0].Body)
		}
	})
	t.Run("external emails", func(t *testing.T) {
		e := newEnv(t, morningBriefing(), func(s *catalog.Stack) { s.Limits = &catalog.Limits{MaxExternalEmails: new(2)} })
		e.eng.Bricks["do.chat"] = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
			for range 3 {
				if err := rc.UseExternalEmail(); err != nil {
					return nil, err
				}
			}
			return in, nil
		})
		if r := e.run(e.start("manual", false)); r.Status != store.RunStopped {
			t.Fatalf("run: %s", r.Status)
		}
	})
}

func TestTestModeCapsPiles(t *testing.T) {
	stack := morningBriefing()
	stack[2].Config = cfg{"criteria": "x", "max": 10}
	e := newEnv(t, stack)
	r := e.run(e.start("test", false))
	if !r.TestMode || r.Status != store.RunDone {
		t.Fatalf("run: %+v", r)
	}
	steps := e.steps(r.ID)
	if steps[1].CardsOut != engine.TestModeMaxCards || len(e.cards(steps[1].ID)) != engine.TestModeMaxCards {
		t.Fatalf("search pile in test mode: %d", steps[1].CardsOut)
	}
}

func TestNothingFoundEndsEarly(t *testing.T) {
	e := newEnv(t, morningBriefing())
	e.eng.Bricks = fakeBricks(e.calls, 0)
	r := e.run(e.start("manual", false))
	if r.Status != store.RunDone || e.calls.get("think.keep") != 0 || e.calls.get("do.chat") != 0 {
		t.Fatalf("run %s; keep %d chat %d", r.Status, e.calls.get("think.keep"), e.calls.get("do.chat"))
	}
	if f := e.feed(r.ID); len(f) != 1 || !strings.Contains(f[0].Title, "Nothing new") {
		t.Fatalf("feed: %+v", f)
	}
}

func TestStackProblemsFailTheRun(t *testing.T) {
	t.Run("invalid stack", func(t *testing.T) {
		stack := morningBriefing()
		stack[4].Config = cfg{} // Post to chat without a chat
		e := newEnv(t, stack)
		r := e.run(e.start("manual", false))
		if r.Status != store.RunFailed || e.calls.get("look.search") != 0 {
			t.Fatalf("run %s", r.Status)
		}
		if f := e.feed(r.ID); !strings.Contains(*f[0].Body, "needs a fix") {
			t.Fatalf("feed: %s", *f[0].Body)
		}
	})
	t.Run("brick without executor", func(t *testing.T) {
		e := newEnv(t, morningBriefing())
		delete(e.eng.Bricks, "do.chat")
		r := e.run(e.start("manual", false))
		if r.Status != store.RunFailed || e.calls.get("look.search") != 0 {
			t.Fatalf("run %s", r.Status)
		}
		if f := e.feed(r.ID); !strings.Contains(*f[0].Body, "Post to chat") {
			t.Fatalf("feed: %s", *f[0].Body)
		}
	})
}

func TestLargeContentGoesToBlobs(t *testing.T) {
	e := newEnv(t, morningBriefing())
	big := strings.Repeat("Bäckerei ", 8000) // ~80 KB
	e.eng.Bricks["look.search"] = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
		var c engine.Card
		c.Set("title", "page", engine.TrustUntrusted)
		c.Set("page_text", big, engine.TrustUntrusted)
		return []engine.Card{c}, nil
	})
	failOnce := true
	e.eng.Bricks["think.keep"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		if failOnce {
			failOnce = false
			return nil, errors.New("transient")
		}
		if in[0].Text("page_text") != big {
			return nil, queue.Permanent(errors.New("page text not restored on resume"))
		}
		return in, nil
	})
	r := e.start("manual", false)
	_ = e.eng.Execute(context.Background(), r.ID) // fails at keep
	stored := e.cards(e.steps(r.ID)[1].ID)
	ref, ok := stored[0].Data["page_text"].(map[string]any)
	if !ok || ref["$blob"] == nil || e.blobs.Len() != 1 {
		t.Fatalf("large field not offloaded: %T", stored[0].Data["page_text"])
	}
	if r = e.run(r); r.Status != store.RunDone {
		t.Fatalf("run: %s %v", r.Status, r.Error)
	}
}

func TestRunJobs(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, morningBriefing())

	// A scheduled job (from enqueue_due_triggers) has no run yet.
	payload, _ := json.Marshal(map[string]any{"trigger_id": "t1", "helper_id": e.helper.ID,
		"version_id": *e.helper.CurrentVersionID, "trigger_type": "schedule", "scheduled_for": "2026-11-10T07:30:00Z"})
	job := queue.Job{ID: store.NewID(), Kind: engine.JobKind, Payload: payload}
	if err := e.eng.HandleJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	r := e.get(job.ID)
	if r.Status != store.RunDone || r.TriggerType != "schedule" || r.TriggerData["scheduled_for"] != "2026-11-10T07:30:00Z" {
		t.Fatalf("scheduled run: %+v", r)
	}
	// The same job again (e.g. redelivered): no second run, nothing re-runs.
	if err := e.eng.HandleJob(ctx, job); err != nil || e.calls.get("look.search") != 1 {
		t.Fatalf("redelivered job: %v, search calls %d", err, e.calls.get("look.search"))
	}

	// An existing run.
	r2 := e.start("manual", false)
	p2, _ := json.Marshal(map[string]string{"run_id": r2.ID})
	if err := e.eng.HandleJob(ctx, queue.Job{ID: store.NewID(), Kind: engine.JobKind, Payload: p2}); err != nil {
		t.Fatal(err)
	}
	if e.get(r2.ID).Status != store.RunDone {
		t.Fatal("run job didn't finish the run")
	}

	// A paused helper's scheduled job is skipped.
	h := e.helper
	h.Status = "paused"
	e.store.PutHelper(h)
	job3 := queue.Job{ID: store.NewID(), Kind: engine.JobKind, Payload: payload}
	if err := e.eng.HandleJob(ctx, job3); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.GetRun(ctx, job3.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("paused helper got a run")
	}
	// But a manual run of a paused helper works.
	if r := e.run(e.start("manual", false)); r.Status != store.RunDone {
		t.Fatalf("manual run of paused helper: %s", r.Status)
	}
}

func TestJobFailedMarksTheRun(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, morningBriefing())
	e.eng.Bricks["think.keep"] = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
		return nil, errors.New("timeout")
	})
	r := e.start("manual", false)
	p, _ := json.Marshal(map[string]string{"run_id": r.ID})
	job := queue.Job{ID: store.NewID(), Kind: engine.JobKind, Payload: p}
	if err := e.eng.HandleJob(ctx, job); err == nil {
		t.Fatal("want the transient error")
	}
	e.eng.JobFailed(ctx, job, errors.New("timeout"))
	got := e.get(r.ID)
	if got.Status != store.RunFailed {
		t.Fatalf("status %s", got.Status)
	}
	if f := e.feed(r.ID); len(f) != 1 || f[0].Type != "problem" {
		t.Fatalf("feed: %+v", f)
	}
	e.eng.JobFailed(ctx, job, errors.New("again"))
	if f := e.feed(r.ID); len(f) != 1 {
		t.Fatal("a finished run must not get a second Problem card")
	}
}

func TestRunValuesAndContext(t *testing.T) {
	stack := []sb{
		{ID: "b1", Type: "when.tap", Config: cfg{"question": "Which city?"}},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakeries in {answer}"}},
	}
	e := newEnv(t, stack)
	var seen string
	e.eng.Bricks["look.search"] = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, _ engine.Config) ([]engine.Card, error) {
		seen, _ = rc.Value("answer")
		if rc.StepID == "" || rc.BrickID != "b2" {
			return nil, queue.Permanent(errors.New("step not set"))
		}
		return nil, nil
	})
	r, err := e.eng.StartRun(context.Background(), engine.StartRequest{HelperID: e.helper.ID, TriggerType: "manual",
		TriggerData: map[string]any{"answer": "Dresden"}})
	if err != nil {
		t.Fatal(err)
	}
	if r = e.run(r); r.Status != store.RunDone || seen != "Dresden" {
		t.Fatalf("run %s, answer %q", r.Status, seen)
	}
}

func TestStartRunRules(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, morningBriefing())
	h := e.helper
	h.Status = "draft"
	e.store.PutHelper(h)
	if _, err := e.eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: "schedule"}); !errors.Is(err, engine.ErrNotRunnable) {
		t.Fatalf("draft helper on schedule: %v", err)
	}
	if r, err := e.eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: "test"}); err != nil || !r.TestMode {
		t.Fatalf("test run of a draft: %v %+v", err, r)
	}
	h.CurrentVersionID = nil
	e.store.PutHelper(h)
	if _, err := e.eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: "manual"}); !errors.Is(err, engine.ErrNotRunnable) {
		t.Fatalf("no version: %v", err)
	}
	// Idempotent by run id.
	h.CurrentVersionID = e.helper.CurrentVersionID
	e.store.PutHelper(h)
	id := store.NewID()
	a, _ := e.eng.StartRun(ctx, engine.StartRequest{RunID: id, HelperID: h.ID, TriggerType: "manual"})
	b, _ := e.eng.StartRun(ctx, engine.StartRequest{RunID: id, HelperID: h.ID, TriggerType: "manual"})
	if a.ID != id || b.ID != id || !a.CreatedAt.Equal(b.CreatedAt) {
		t.Fatal("StartRun with the same id must return the same run")
	}
}

// A pill without a value ends the run with a Problem card that says which.
func TestMissingValueEndsTheRun(t *testing.T) {
	e := newEnv(t, morningBriefing())
	e.eng.Bricks["think.summarize"] = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
		return nil, fmt.Errorf("What to write: %w", &missingErr{})
	})
	r := e.run(e.start("manual", false))
	if r.Status != store.RunFailed {
		t.Fatalf("status %s", r.Status)
	}
	if f := e.feed(r.ID); len(f) != 1 || !strings.Contains(*f[0].Body, "{my offer} isn't set") {
		t.Fatalf("feed: %+v", f)
	}
}

type missingErr struct{}

func (*missingErr) Error() string { return "{my offer} isn't set yet" }
func (*missingErr) Unwrap() error { return engine.ErrMissingValue }

// ---- the brain ----

type fakeChat struct{ answers []string }

func (f *fakeChat) Chat(context.Context, llm.Request) (llm.Response, error) {
	text := `{"summary":"ok"}`
	if len(f.answers) > 0 {
		text, f.answers = f.answers[0], f.answers[1:]
	}
	return llm.Response{Text: text, Model: "mistralai/mistral-small-3.2-24b-instruct", TokensIn: 900, TokensOut: 100,
		CostUSD: 0.0001, FinishReason: "stop"}, nil
}

// summarizeWithBrain is a Sum it up that really asks the (fake) brain.
var summarizeWithBrain = engine.BrickFunc(func(ctx context.Context, rc *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
	var out struct {
		Summary string `json:"summary"`
	}
	if err := rc.Brain.AskJSON(ctx, rc, llm.Request{Messages: []llm.Message{{Role: "user", Content: "sum up"}}}, &out, nil); err != nil {
		return nil, err
	}
	var c engine.Card
	c.Set("summary", out.Summary, engine.TrustExtracted)
	return []engine.Card{c}, nil
})

func withBrain(t *testing.T, answers ...string) *env {
	t.Helper()
	e := newEnv(t, morningBriefing())
	e.store.PutBrain(store.Brain{WorkspaceID: e.ws.ID, UserID: e.ws.UserID, AllowanceCredits: 500})
	e.eng.Brain = &llm.Brain{Chat: &fakeChat{answers: answers}, Store: e.store, USDToEUR: 1}
	e.eng.Bricks["think.summarize"] = summarizeWithBrain
	return e
}

func TestBrainIsMeteredPerRun(t *testing.T) {
	e := withBrain(t)
	r := e.run(e.start("manual", false))
	if r.Status != store.RunDone || r.TokensIn != 900 || r.TokensOut != 100 {
		t.Fatalf("run: %s tokens %d/%d", r.Status, r.TokensIn, r.TokensOut)
	}
	usage := e.store.Usage()
	if len(usage) != 1 || usage[0].Kind != "llm" || usage[0].RunID == nil || *usage[0].RunID != r.ID ||
		usage[0].WorkspaceID != e.ws.ID || float64(usage[0].BrainCredits) != 0.1 {
		t.Fatalf("ledger: %+v", usage)
	}
	b, _ := e.store.GetBrain(context.Background(), e.ws.ID)
	if float64(b.UsedCreditsThisPeriod) != 0.1 {
		t.Fatalf("allowance: %v", b.UsedCreditsThisPeriod)
	}
}

func TestAllowanceStopsTheRun(t *testing.T) {
	e := withBrain(t)
	if _, err := e.store.AddBrainUsage(context.Background(), e.ws.ID, 500); err != nil {
		t.Fatal(err)
	}
	r := e.run(e.start("manual", false))
	if r.Status != store.RunStopped || e.calls.get("do.chat") != 0 {
		t.Fatalf("run %s", r.Status)
	}
	if f := e.feed(r.ID); len(f) != 1 || !strings.Contains(*f[0].Body, "allowance") || !strings.Contains(*f[0].Body, "Upgrade") {
		t.Fatalf("feed: %+v", f)
	}
}

func TestUnusableAnswerFailsTheRun(t *testing.T) {
	e := withBrain(t, "not json", "still not json")
	r := e.run(e.start("manual", false))
	if r.Status != store.RunFailed {
		t.Fatalf("run %s", r.Status)
	}
	if len(e.store.Usage()) != 2 {
		t.Fatal("both attempts must be in the ledger")
	}
}

// ---- Ask me helpers ----

// waiting returns the Feed card a waiting run waits on.
func (e *env) waiting(r store.Run) store.FeedCard {
	e.t.Helper()
	r = e.get(r.ID)
	if r.Status != store.RunWaiting || r.WaitingOnCardID == nil {
		e.t.Fatalf("run is %s, not waiting", r.Status)
	}
	fc, err := e.store.GetFeedCard(context.Background(), *r.WaitingOnCardID)
	if err != nil {
		e.t.Fatal(err)
	}
	return fc
}

// decide records a decision on the waiting Feed card (as the API does) and
// runs the job that resumes the run.
func (e *env) decide(r store.Run, state string, d engine.Decision) store.Run {
	e.t.Helper()
	fc := e.waiting(r)
	if err := engine.CheckDecision(fc, e.cards(fc.ID), state, d); err != nil {
		e.t.Fatal(err)
	}
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, err := e.store.UpdateFeedCard(context.Background(), fc.ID, store.FeedCardPatch{State: &state, Decision: m}); err != nil {
		e.t.Fatal(err)
	}
	return e.run(r)
}

func cardIDs(cards []store.Card) []string {
	var out []string
	for _, c := range cards {
		out = append(out, c.CardID)
	}
	return out
}

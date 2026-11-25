package engine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
)

// learnStack: Keep the good ones, then Let me pick (#76).
func learnStack(remember bool) []sb {
	return []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakery news"}},
		{ID: "b3", Type: "think.keep", Config: cfg{"criteria": "news for bakeries", "max": 5}},
		{ID: "b4", Type: "ask.pick", Config: cfg{"remember": remember}},
	}
}

// keepSees records what Keep the good ones learned before each run.
func keepSees(e *env) *[][]store.PickExample {
	var seen [][]store.PickExample
	e.eng.Bricks["think.keep"] = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		seen = append(seen, rc.Learned)
		return in, nil
	})
	return &seen
}

func TestLearnFromSwipes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, learnStack(true))
	seen := keepSees(e)

	// First run: nothing learned yet. The user keeps one of five.
	r := e.run(e.start("manual", false))
	fc := e.waiting(r)
	ids := cardIDs(e.cards(fc.ID))
	e.decide(r, engine.StateAnswered, engine.Decision{Picked: []string{ids[2]}})
	if len((*seen)[0]) != 0 {
		t.Fatalf("first run learned %+v", (*seen)[0])
	}
	if n, _ := e.store.CountPickExamples(ctx, e.helper.ID); n != 5 {
		t.Fatalf("remembered %d swipes", n)
	}

	// Second run: Keep sees the choice, kept ones first, with the site.
	r = e.run(e.start("manual", false))
	got := (*seen)[1]
	if len(got) != 5 || !got[0].Kept || got[0].Title != "bakery news #2" || got[0].Site != "example.com" || got[1].Kept {
		t.Fatalf("learned %+v", got)
	}
	// A test run's swipes aren't learned.
	e.decide(r, engine.StateAnswered, engine.Decision{Picked: []string{}})
	tr := e.run(e.start("manual", true))
	e.decide(tr, engine.StateAnswered, engine.Decision{})
	if n, _ := e.store.CountPickExamples(ctx, e.helper.ID); n != 10 {
		t.Fatalf("after a test run: %d swipes", n)
	}

	// Forget: the next run starts fresh.
	if err := e.store.ForgetPickExamples(ctx, e.helper.ID); err != nil {
		t.Fatal(err)
	}
	e.run(e.start("manual", false))
	if last := (*seen)[len(*seen)-1]; len(last) != 0 {
		t.Fatalf("after Forget: %+v", last)
	}
}

// With "Learn from my choices" off, nothing is remembered or used.
func TestLearnOff(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, learnStack(false))
	seen := keepSees(e)
	_ = e.store.AddPickExamples(ctx, []store.PickExample{{HelperID: e.helper.ID, StepID: store.NewID(), CardID: "x", Kept: true, Title: "old"}})
	r := e.run(e.start("manual", false))
	ids := cardIDs(e.cards(e.waiting(r).ID))
	e.decide(r, engine.StateAnswered, engine.Decision{Picked: ids[:1]})
	if len((*seen)[0]) != 0 {
		t.Fatalf("used %+v", (*seen)[0])
	}
	if n, _ := e.store.CountPickExamples(ctx, e.helper.ID); n != 1 {
		t.Fatalf("remembered with the setting off: %d", n)
	}
}

// Without a Keep the good ones before it, nothing would use the swipes, so
// none are kept.
func TestLearnNeedsKeep(t *testing.T) {
	e := newEnv(t, []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "events"}},
		{ID: "b3", Type: "ask.pick"},
	})
	r := e.run(e.start("manual", false))
	ids := cardIDs(e.cards(e.waiting(r).ID))
	e.decide(r, engine.StateAnswered, engine.Decision{Picked: ids[:1]})
	if n, _ := e.store.CountPickExamples(context.Background(), e.helper.ID); n != 0 {
		t.Fatalf("remembered %d swipes nothing uses", n)
	}
}

// A run that saved its cards to a list says which on its result card, so
// the Feed doesn't offer "Add to list" too.
func TestResultSavedTo(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, []sb{{ID: "b1", Type: "when.tap"}})
	l := store.List{WorkspaceID: e.ws.ID, UserID: e.ws.UserID, Name: "Weekender"}
	if err := e.store.CreateList(ctx, &l); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.tap"}, {ID: "b2", Type: "look.search", Config: map[string]any{"query": "events"}},
		{ID: "b3", Type: "do.list", Config: map[string]any{"list": l.ID, "dedupe": "url"}},
	}})
	v := e.store.PutHelperVersion(store.HelperVersion{WorkspaceID: e.ws.ID, UserID: e.ws.UserID, HelperID: e.helper.ID, Version: 2, Stack: raw})
	e.helper.CurrentVersionID = &v.ID
	e.helper = e.store.PutHelper(e.helper)
	e.eng.Bricks["do.list"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		return in, nil
	})
	r := e.run(e.start("manual", false))
	for _, f := range e.feed(r.ID) {
		if f.Type == "result" {
			saved, _ := f.Payload["saved_to"].(map[string]any)
			if saved["name"] != "Weekender" || saved["id"] != l.ID {
				t.Fatalf("result payload %+v", f.Payload)
			}
			return
		}
	}
	t.Fatal("no result card")
}

// A run that ends at Post to social says on its result card what became of
// the post, not just "Done".
func TestResultSaysWhatBecameOfThePost(t *testing.T) {
	e := newEnv(t, []sb{{ID: "b1", Type: "when.tap"}})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.tap"}, {ID: "b2", Type: "think.write", Config: map[string]any{"what": "a post about bread", "mode": "all"}},
		{ID: "b3", Type: "do.social", Config: map[string]any{"channels": []any{"11"}, "when": "draft"}},
	}})
	v := e.store.PutHelperVersion(store.HelperVersion{WorkspaceID: e.ws.ID, UserID: e.ws.UserID, HelperID: e.helper.ID, Version: 2, Stack: raw})
	e.helper.CurrentVersionID = &v.ID
	e.helper = e.store.PutHelper(e.helper)
	e.eng.Bricks["think.write"] = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
		c := engine.Card{Kind: "draft"}
		c.Set("body", "Fresh sourdough every Friday", engine.TrustExtracted)
		return []engine.Card{c}, nil
	})
	e.eng.Bricks["do.social"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		in[0].Set("posted", "Saved as a draft in Posta", engine.TrustUser)
		return in, nil
	})
	r := e.run(e.start("manual", false))
	for _, f := range e.feed(r.ID) {
		if f.Type == "result" {
			if f.Title != "Saved as a draft in Posta" || f.Body == nil || *f.Body != "Fresh sourdough every Friday" {
				t.Fatalf("result card %q %v", f.Title, f.Body)
			}
			return
		}
	}
	t.Fatalf("no result card: %+v", e.feed(r.ID))
}

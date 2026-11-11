package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/store"
)

func checkStack(onTimeout string) []sb {
	return []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakery news"}},
		{ID: "b3", Type: "ask.check", Config: cfg{"on_timeout": onTimeout}},
		{ID: "b4", Type: "do.chat", Config: cfg{"place": "p1"}},
	}
}

// chatSeen records the pile Post to chat gets.
func chatSeen(e *env) *[]engine.Card {
	var seen []engine.Card
	e.eng.Bricks["do.chat"] = engine.BrickFunc(func(_ context.Context, _ *engine.RunContext, in []engine.Card, _ engine.Config) ([]engine.Card, error) {
		seen = append(seen, in...)
		return in, nil
	})
	return &seen
}

// Pause → skip and edit → resume gives the expected pile; nothing stays in
// memory between the two jobs.
func TestPauseCheckSkipEdit(t *testing.T) {
	e := newEnv(t, checkStack("skip"))
	seen := chatSeen(e)
	r := e.run(e.start("manual", false))
	if r.Status != store.RunWaiting || r.NextBrickIndex != 2 {
		t.Fatalf("run %s at %d", r.Status, r.NextBrickIndex)
	}
	fc := e.waiting(r)
	if fc.ID != *fc.StepID || fc.Type != engine.FeedApproval || fc.State != engine.StateOpen || fc.ExpiresAt == nil ||
		fc.Body == nil || *fc.Body != "Please check 5 before they go on." || fc.Payload["brick_id"] != "b3" {
		t.Fatalf("feed card %+v", fc)
	}
	if d := time.Until(*fc.ExpiresAt); d < 47*time.Hour || d > 49*time.Hour {
		t.Fatalf("expires in %v, want 48h", d)
	}
	shown := e.cards(fc.ID)
	if len(shown) != 5 {
		t.Fatalf("%d cards shown", len(shown))
	}

	// Another job for the waiting run (the sweep, a retry) changes nothing.
	r = e.run(r)
	if r.Status != store.RunWaiting || len(e.steps(r.ID)) != 3 || len(e.feed(r.ID)) != 1 || len(*seen) != 0 {
		t.Fatalf("second job: %s, %d steps, %d feed cards", r.Status, len(e.steps(r.ID)), len(e.feed(r.ID)))
	}

	ids := cardIDs(shown)
	r = e.decide(r, engine.StateEdited, engine.Decision{
		Skip:  []string{ids[1], ids[3]},
		Edits: map[string]map[string]any{ids[2]: {"title": "Flour prices (checked)"}},
	})
	if r.Status != store.RunDone {
		t.Fatalf("status %s", r.Status)
	}
	got := *seen
	if len(got) != 3 || got[0].ID != ids[0] || got[1].ID != ids[2] || got[2].ID != ids[4] {
		t.Fatalf("pile after Check first: %+v", got)
	}
	if got[1].Text("title") != "Flour prices (checked)" || got[1].TrustOf("title") != engine.TrustUser {
		t.Fatalf("edit: %+v", got[1])
	}
	if got[0].TrustOf("title") != engine.TrustUntrusted { // raw search text stays untrusted
		t.Fatalf("approval must not upgrade untrusted fields: %v", got[0].Trust)
	}
	var check store.Step
	for _, s := range e.steps(r.ID) {
		if s.BrickID == "b3" {
			check = s
		}
	}
	if check.Status != "done" || check.CardsIn != 5 || check.CardsOut != 3 || len(check.Log) != 1 || check.Log[0]["code"] != "decision" {
		t.Fatalf("check step %+v", check)
	}
	if res := e.feed(r.ID); len(res) != 2 || res[1].Type != "result" {
		t.Fatalf("feed: %+v", res)
	}
}

func TestPauseSkipAllEndsQuietly(t *testing.T) {
	e := newEnv(t, checkStack("skip"))
	seen := chatSeen(e)
	r := e.run(e.start("manual", false))
	r = e.decide(r, engine.StateSkipped, engine.Decision{Skip: cardIDs(e.cards(e.waiting(r).ID))})
	if r.Status != store.RunDone || len(*seen) != 0 {
		t.Fatalf("status %s, chat got %d", r.Status, len(*seen))
	}
}

// An unanswered Check first expires: "skip" drops everything, "go" goes
// on without approving anything.
func TestPauseExpires(t *testing.T) {
	for _, onTimeout := range []string{"skip", "go"} {
		t.Run(onTimeout, func(t *testing.T) {
			e := newEnv(t, checkStack(onTimeout))
			seen := chatSeen(e)
			r := e.run(e.start("manual", false))
			e.eng.Now = func() time.Time { return time.Now().Add(47 * time.Hour) }
			if r = e.run(r); r.Status != store.RunWaiting {
				t.Fatalf("before expiry: %s", r.Status)
			}
			fcID := e.waiting(r).ID
			e.eng.Now = func() time.Time { return time.Now().Add(49 * time.Hour) }
			r = e.run(r)
			fc, _ := e.store.GetFeedCard(context.Background(), fcID)
			if r.Status != store.RunDone || fc.State != engine.StateExpired {
				t.Fatalf("run %s, card %s", r.Status, fc.State)
			}
			want := map[string]int{"skip": 0, "go": 5}[onTimeout]
			if len(*seen) != want {
				t.Fatalf("chat got %d cards, want %d", len(*seen), want)
			}
			for _, c := range *seen {
				if c.TrustOf("title") == engine.TrustApproved || c.TrustOf("title") == engine.TrustUser {
					t.Fatal("an unanswered check must not approve anything")
				}
			}
		})
	}
}

func TestPausePick(t *testing.T) {
	e := newEnv(t, []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakery news"}},
		{ID: "b3", Type: "ask.pick"},
		{ID: "b4", Type: "do.chat", Config: cfg{"place": "p1"}},
	})
	seen := chatSeen(e)
	r := e.run(e.start("manual", false))
	fc := e.waiting(r)
	if fc.Type != engine.FeedPick || fmt.Sprint(fc.Payload["max_show"]) != "10" {
		t.Fatalf("feed card %+v", fc)
	}
	ids := cardIDs(e.cards(fc.ID))
	r = e.decide(r, engine.StateAnswered, engine.Decision{Picked: []string{ids[3], ids[0]}})
	if r.Status != store.RunDone || len(*seen) != 2 || (*seen)[0].ID != ids[0] || (*seen)[1].ID != ids[3] {
		t.Fatalf("status %s, picked pile %+v", r.Status, *seen)
	}
}

// Let me pick as the last brick (Weekend Scout): the result card counts
// what was picked, not everything shown.
func TestPickLastResult(t *testing.T) {
	e := newEnv(t, []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "events"}},
		{ID: "b3", Type: "ask.pick"},
	})
	r := e.run(e.start("manual", false))
	fc := e.waiting(r)
	ids := cardIDs(e.cards(fc.ID))
	r = e.decide(r, engine.StateAnswered, engine.Decision{Picked: []string{ids[1], ids[2]}})
	var result *store.FeedCard
	for _, f := range e.feed(r.ID) {
		if f.Type == "result" {
			result = &f
		}
	}
	if result == nil || result.Title != "2 new cards" || fmt.Sprint(result.Payload["cards"]) != "2" {
		t.Fatalf("result %+v", result)
	}
}

// Let me pick shows at most max_show cards; the rest don't go on.
func TestPausePickMaxShow(t *testing.T) {
	e := newEnv(t, []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: cfg{"query": "bakery news"}},
		{ID: "b3", Type: "ask.pick", Config: cfg{"max_show": 2}},
		{ID: "b4", Type: "do.chat", Config: cfg{"place": "p1"}},
	})
	r := e.run(e.start("manual", false))
	fc := e.waiting(r)
	if n := len(e.cards(fc.ID)); n != 2 || fmt.Sprint(fc.Payload["cards"]) != "2" {
		t.Fatalf("shown %d, payload %v", n, fc.Payload)
	}
}

// Ask me something at the top: the answer fills {answer} further down.
func TestPauseQuestion(t *testing.T) {
	e := newEnv(t, []sb{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "ask.question", Config: cfg{"question": "Which city near {my city}?", "answers": []any{"Leipzig", "Dresden"}, "free_text": false}},
		{ID: "b3", Type: "look.search", Config: cfg{"query": "bakery news {answer}"}},
		{ID: "b4", Type: "do.chat", Config: cfg{"place": "p1"}},
	})
	e.eng.Resolve = tmpl.Config
	var asked string
	e.eng.Bricks["look.search"] = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, _ engine.Config) ([]engine.Card, error) {
		asked, _ = rc.Value("answer")
		var c engine.Card
		c.Set("title", "news for "+asked, engine.TrustUntrusted)
		return []engine.Card{c}, nil
	})
	r := e.run(e.start("manual", false))
	fc := e.waiting(r)
	if fc.Type != engine.FeedQuestion || *fc.Body != "Which city near Leipzig?" || fc.Payload["question"] != "Which city near Leipzig?" {
		t.Fatalf("feed card %+v", fc)
	}
	err := engine.CheckDecision(fc, nil, engine.StateAnswered, engine.Decision{Answer: "Berlin"})
	if !errors.Is(err, engine.ErrBadDecision) {
		t.Fatalf("an answer that isn't offered: %v", err)
	}
	r = e.decide(r, engine.StateAnswered, engine.Decision{Answer: "Dresden"})
	if r.Status != store.RunDone || asked != "Dresden" {
		t.Fatalf("status %s, search got %q", r.Status, asked)
	}
}

func TestCheckDecision(t *testing.T) {
	fc := store.FeedCard{Type: engine.FeedApproval}
	shown := []store.Card{{CardID: "a"}, {CardID: "b"}}
	for _, c := range []struct {
		state string
		d     engine.Decision
		ok    bool
	}{
		{engine.StateApproved, engine.Decision{}, true},
		{engine.StateApproved, engine.Decision{Skip: []string{"b"}}, true},
		{engine.StateEdited, engine.Decision{Edits: map[string]map[string]any{"a": {"email": "x@y.example"}}}, true},
		{engine.StateApproved, engine.Decision{Edits: map[string]map[string]any{"a": {"email": "x@y.example"}}}, false},
		{engine.StateApproved, engine.Decision{Skip: []string{"zzz"}}, false},
		{engine.StateApproved, engine.Decision{Skip: []string{"a", "a"}}, false},
		{engine.StateEdited, engine.Decision{Skip: []string{"a"}, Edits: map[string]map[string]any{"a": {"t": "x"}}}, false},
		{engine.StateEdited, engine.Decision{Edits: map[string]map[string]any{"a": {"$blob": "x"}}}, false},
		{engine.StateEdited, engine.Decision{Edits: map[string]map[string]any{"a": {"n": 3.0}}}, false},
		{engine.StateAnswered, engine.Decision{Answer: "yes"}, false},
	} {
		if err := engine.CheckDecision(fc, shown, c.state, c.d); (err == nil) != c.ok {
			t.Errorf("%s %+v: %v", c.state, c.d, err)
		}
	}
}

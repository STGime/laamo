package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// Ask me bricks that wait for the user (#20). A run pauses at one of them:
// the step keeps the pile the user sees, a Feed card asks, and the run is
// "waiting" with no job and nothing in memory. When the user decides (or
// the card expires), a run job resumes it: the decision is recorded in the
// step's log, and every later load of the pile replays it.

// Feed card types and states (feed_cards.type / .state).
const (
	FeedApproval = "approval"
	FeedPick     = "pick"
	FeedQuestion = "question"

	StateOpen     = "open"
	StateApproved = "approved"
	StateEdited   = "edited"
	StateSkipped  = "skipped"
	StateAnswered = "answered"
	StateExpired  = "expired"
	StateDismiss  = "dismissed"
)

// Pauses reports whether a brick type waits for the user.
func Pauses(brickType string) bool {
	return brickType == "ask.check" || brickType == "ask.pick" || brickType == "ask.question"
}

func feedType(brickType string) string {
	switch brickType {
	case "ask.pick":
		return FeedPick
	case "ask.question":
		return FeedQuestion
	}
	return FeedApproval
}

// Decision is what the user sent for an Ask me Feed card
// (feed_cards.decision).
type Decision struct {
	// Check first: cards to leave out, and fields the user changed (card
	// id → field → value). Every other card is approved.
	Skip  []string                  `json:"skip,omitempty"`
	Edits map[string]map[string]any `json:"edits,omitempty"`
	// Let me pick: the chosen cards.
	Picked []string `json:"picked,omitempty"`
	// Ask me something: the answer.
	Answer string `json:"answer,omitempty"`
}

// ErrBadDecision: the decision doesn't fit the card (unknown card ids, an
// answer that isn't offered, …).
var ErrBadDecision = errors.New("bad decision")

// MaxEditChars caps one edited field.
const MaxEditChars = 20000

// CheckDecision validates a decision for a Feed card whose step showed
// shown. state is the state the card will get (approved, edited, skipped,
// answered).
func CheckDecision(fc store.FeedCard, shown []store.Card, state string, d Decision) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrBadDecision, fmt.Sprintf(format, a...))
	}
	ids := map[string]bool{}
	for _, c := range shown {
		ids[c.CardID] = true
	}
	known := func(list []string) error {
		seen := map[string]bool{}
		for _, id := range list {
			if !ids[id] || seen[id] {
				return bad("card %q isn't in this pile or is listed twice", id)
			}
			seen[id] = true
		}
		return nil
	}
	switch fc.Type {
	case FeedApproval:
		if !slices.Contains([]string{StateApproved, StateEdited, StateSkipped}, state) {
			return bad("a Check first card can be approved, edited or skipped")
		}
		if d.Answer != "" || len(d.Picked) > 0 {
			return bad("a Check first card takes skip and edits only")
		}
		if err := known(d.Skip); err != nil {
			return err
		}
		for id, fields := range d.Edits {
			if !ids[id] {
				return bad("card %q isn't in this pile", id)
			}
			if slices.Contains(d.Skip, id) {
				return bad("card %q is skipped and edited", id)
			}
			for f, v := range fields {
				s, ok := v.(string)
				if !ok || f == "" || strings.HasPrefix(f, "$") || len([]rune(s)) > MaxEditChars {
					return bad("edit %s.%s must be text up to %d characters", id, f, MaxEditChars)
				}
			}
		}
		if (state == StateEdited) != (len(d.Edits) > 0) {
			return bad("state %q doesn't match the edits", state)
		}
	case FeedPick:
		if state != StateAnswered || d.Answer != "" || len(d.Skip) > 0 || len(d.Edits) > 0 {
			return bad("a Let me pick card takes picked cards only")
		}
		return known(d.Picked)
	case FeedQuestion:
		if state != StateAnswered || len(d.Picked) > 0 || len(d.Skip) > 0 || len(d.Edits) > 0 {
			return bad("an Ask me something card takes an answer only")
		}
		answer := strings.TrimSpace(d.Answer)
		if answer == "" || len([]rune(answer)) > 2000 {
			return bad("the answer must be 1 to 2000 characters")
		}
		answers, _ := fc.Payload["answers"].([]any)
		free, _ := fc.Payload["free_text"].(bool)
		if len(answers) > 0 && !free && !slices.ContainsFunc(answers, func(a any) bool { return a == answer }) {
			return bad("pick one of the offered answers")
		}
	default:
		return bad("this card doesn't take a decision")
	}
	return nil
}

// applied is a decision in the form the engine replays (step log entry
// "decision"): which cards go on, whether they count as approved, the
// user's edits, and an answer.
type applied struct {
	Keep    []string                  `json:"keep"`
	Approve bool                      `json:"approve,omitempty"`
	Edits   map[string]map[string]any `json:"edits,omitempty"`
	Answer  string                    `json:"answer,omitempty"`
	State   string                    `json:"state"`
}

// settle turns a decided (or expired) Feed card into what happens next.
func settle(brickType string, cfg Config, shown []Card, fc store.FeedCard) (applied, error) {
	var d Decision
	if fc.Decision != nil {
		raw, _ := json.Marshal(fc.Decision)
		if err := json.Unmarshal(raw, &d); err != nil {
			return applied{}, fmt.Errorf("%w: %w", ErrBadDecision, err)
		}
	}
	a := applied{State: fc.State, Keep: []string{}}
	all := func() {
		for _, c := range shown {
			a.Keep = append(a.Keep, c.ID)
		}
	}
	switch brickType {
	case "ask.check":
		switch fc.State {
		case StateApproved, StateEdited:
			for _, c := range shown {
				if !slices.Contains(d.Skip, c.ID) {
					a.Keep = append(a.Keep, c.ID)
				}
			}
			a.Approve, a.Edits = true, d.Edits
		case StateExpired:
			// Unanswered: skip, or go ahead WITHOUT approval (the validator
			// only allows going ahead when nothing goes to card recipients).
			if cfg.String("on_timeout") == "go" {
				all()
			}
		}
	case "ask.pick":
		for _, c := range shown { // keep pile order
			if slices.Contains(d.Picked, c.ID) {
				a.Keep = append(a.Keep, c.ID)
			}
		}
	case "ask.question":
		all()
		a.Answer = strings.TrimSpace(d.Answer)
	}
	return a, nil
}

// replay applies a settled decision to the pile the user saw.
func (a applied) replay(shown []Card) []Card {
	var out []Card
	for _, c := range shown {
		if !slices.Contains(a.Keep, c.ID) {
			continue
		}
		c = c.Clone()
		if c.Trust == nil {
			c.Trust = map[string]string{}
		}
		if a.Approve {
			c.Approve(a.Edits[c.ID])
		}
		out = append(out, c)
	}
	return out
}

// stepDecision finds the decision recorded in an Ask step's log.
func stepDecision(s store.Step) (applied, bool) {
	for _, entry := range s.Log {
		if entry["code"] != "decision" {
			continue
		}
		raw, _ := json.Marshal(entry["applied"])
		var a applied
		if json.Unmarshal(raw, &a) == nil {
			return a, true
		}
	}
	return applied{}, false
}

// errPaused ends Execute without finishing the run.
var errPaused = errors.New("waiting for the user")

// pause stores the pile the user will see, creates the Feed card (its id
// is the step id, so a retried job can't make a second one) and sets the
// run waiting. It is safe to repeat.
func (e *Engine) pause(ctx context.Context, rc *RunContext, r *store.Run, steps []store.Step, i int,
	sb catalog.StackBrick, b *catalog.Brick, pile []Card) error {
	var step store.Step
	for _, s := range steps {
		if s.BrickID == sb.ID && s.Status == "waiting" && s.Attempt >= step.Attempt {
			step = s
		}
	}
	if step.ID == "" {
		attempt := 1
		for _, s := range steps {
			if s.BrickID == sb.ID && s.Attempt >= attempt {
				attempt = s.Attempt + 1
			}
		}
		step = store.Step{WorkspaceID: r.WorkspaceID, UserID: r.UserID, RunID: r.ID, BrickID: sb.ID,
			BrickType: sb.Type, Position: i, Attempt: attempt, Status: "waiting", CardsIn: len(pile)}
		if err := e.Store.CreateStep(ctx, &step); err != nil {
			return err
		}
	}
	rc.StepID, rc.BrickID, rc.Brick = step.ID, sb.ID, b
	cfg := Config(b.Config(sb.Config))
	// Let me pick shows at most max_show cards; the rest don't go on.
	if n := cfg.Int("max_show"); sb.Type == "ask.pick" && n > 0 && len(pile) > n {
		pile = pile[:n]
	}
	if existing, err := e.Store.ListCards(ctx, step.ID); err != nil {
		return err
	} else if len(existing) == 0 && len(pile) > 0 {
		if err := e.saveCards(ctx, rc, step, pile); err != nil {
			return err
		}
	}

	if e.Resolve != nil {
		if resolved, err := e.Resolve(rc, b, cfg, nil); err == nil {
			cfg = resolved
		}
	}
	fc := store.FeedCard{ID: step.ID, WorkspaceID: r.WorkspaceID, UserID: r.UserID, HelperID: &r.HelperID,
		RunID: &r.ID, StepID: &step.ID, Type: feedType(sb.Type), Title: rc.Helper.Name,
		Body: new(askText(sb.Type, cfg, len(pile), rc.Workspace.Locale)), Payload: askPayload(sb, cfg, len(pile))}
	if h := cfg.Int("timeout_h"); sb.Type == "ask.check" && h > 0 {
		fc.ExpiresAt = new(e.now().Add(time.Duration(h) * time.Hour))
	} else if sb.Type != "ask.check" {
		fc.ExpiresAt = new(e.now().Add(7 * 24 * time.Hour))
	}
	if err := e.Store.CreateFeedCard(ctx, &fc); err != nil && !errors.Is(err, store.ErrConflict) {
		return err
	}
	_, err := e.Store.UpdateRun(ctx, r.ID, store.RunPatch{Status: new(store.RunWaiting), NextBrickIndex: &i,
		WaitingOnCardID: store.Set(step.ID), CostEUR: new(store.Num(rc.Cost()))})
	if err != nil {
		return err
	}
	rc.Log.Info("run waiting for the user", "brick", sb.ID, "feed_card", step.ID)
	return errPaused
}

// resume continues a waiting run once its Feed card is decided or expired.
// It reports false when the run should keep waiting.
func (e *Engine) resume(ctx context.Context, r *store.Run, stack catalog.Stack) (bool, error) {
	i := r.NextBrickIndex
	if r.WaitingOnCardID == nil || i >= len(stack.Bricks) || !Pauses(stack.Bricks[i].Type) {
		e.log().Warn("waiting run without an Ask me brick to resume", "run", r.ID)
		return false, nil
	}
	sb := stack.Bricks[i]
	b, _ := e.Catalog.Brick(sb.Type)
	fc, err := e.Store.GetFeedCard(ctx, *r.WaitingOnCardID)
	if err != nil {
		return false, err
	}
	if fc.State == StateOpen {
		if fc.ExpiresAt == nil || e.now().Before(*fc.ExpiresAt) {
			return false, nil // still waiting
		}
		now := e.now()
		if fc, err = e.Store.UpdateFeedCard(ctx, fc.ID, store.FeedCardPatch{State: new(StateExpired), DecidedAt: &now}); err != nil {
			return false, err
		}
	}

	steps, err := e.Store.ListSteps(ctx, r.ID)
	if err != nil {
		return false, err
	}
	var step *store.Step
	for k := range steps {
		if steps[k].ID == fc.ID {
			step = &steps[k]
		}
	}
	if step == nil {
		return false, fmt.Errorf("run %s: no step for Feed card %s", r.ID, fc.ID)
	}
	if step.Status == "waiting" {
		shown, err := e.loadCards(ctx, step.ID)
		if err != nil {
			return false, err
		}
		a, err := settle(sb.Type, Config(b.Config(sb.Config)), shown, fc)
		if err != nil {
			return false, err
		}
		entry := map[string]any{"level": "info", "code": "decision", "applied": a,
			"msg": fmt.Sprintf("%s: %d of %d cards go on", fc.State, len(a.Keep), len(shown))}
		done := e.now()
		if _, err := e.Store.UpdateStep(ctx, step.ID, store.StepPatch{Status: new("done"), CardsOut: new(len(a.Keep)),
			FinishedAt: &done, Log: append(step.Log, entry)}); err != nil {
			return false, err
		}
	}
	next := i + 1
	*r, err = e.Store.UpdateRun(ctx, r.ID, store.RunPatch{Status: new(store.RunRunning), NextBrickIndex: &next,
		WaitingOnCardID: store.Null[string]()})
	return err == nil, err
}

func askPayload(sb catalog.StackBrick, cfg Config, n int) map[string]any {
	p := map[string]any{"brick_id": sb.ID, "brick_type": sb.Type, "cards": n}
	switch sb.Type {
	case "ask.check":
		p["on_timeout"] = cfg.String("on_timeout")
	case "ask.pick":
		p["max_show"] = cfg.Int("max_show")
	case "ask.question":
		p["question"] = cfg.String("question")
		if a, ok := cfg["answers"].([]any); ok {
			p["answers"] = a
		}
		p["free_text"] = cfg.Bool("free_text")
	}
	return p
}

func askText(brickType string, cfg Config, n int, lang string) string {
	de := lang == "de"
	switch brickType {
	case "ask.pick":
		if de {
			return fmt.Sprintf("Wähle aus, was weitergehen soll (%d zur Auswahl).", n)
		}
		return fmt.Sprintf("Pick the ones that should go on (%d to choose from).", n)
	case "ask.question":
		return cfg.String("question")
	}
	if de {
		return fmt.Sprintf("Bitte prüfe %d, bevor es weitergeht.", n)
	}
	return fmt.Sprintf("Please check %d before they go on.", n)
}

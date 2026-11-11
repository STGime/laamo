package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// JobKind is the queue job kind for runs.
const JobKind = "run"

// Engine runs stacks. Bricks maps catalog brick types to executors; a
// stack with a brick that has no executor yet fails at the start of the run.
type Engine struct {
	Store   store.Store
	Catalog *catalog.Catalog
	Bricks  map[string]Brick
	Blobs   Blobs      // nil: large content stays inline
	Brain   *llm.Brain // the AI for Think bricks
	Log     *slog.Logger
	Now     func() time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) log() *slog.Logger {
	if e.Log != nil {
		return e.Log
	}
	return slog.Default()
}

// offloadBytes: string fields larger than this go to Blobs.
const offloadBytes = 32 << 10

// StartRequest starts a run of a helper.
type StartRequest struct {
	RunID       string // optional; the job id for scheduled runs, so a retried job finds its run
	HelperID    string
	VersionID   string // optional; default the helper's current version
	TriggerType string // schedule, manual, email, watch, webhook, test
	TriggerData map[string]any
	TestMode    bool
}

// ErrNotRunnable: the helper can't run (not active, no version).
var ErrNotRunnable = errors.New("helper can't run")

// StartRun creates the run row (status queued). It is idempotent for a
// given RunID.
func (e *Engine) StartRun(ctx context.Context, req StartRequest) (store.Run, error) {
	if req.RunID != "" {
		if r, err := e.Store.GetRun(ctx, req.RunID); err == nil {
			return r, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return store.Run{}, err
		}
	}
	h, err := e.Store.GetHelper(ctx, req.HelperID)
	if err != nil {
		return store.Run{}, err
	}
	automatic := req.TriggerType != "manual" && req.TriggerType != "test"
	if automatic && h.Status != "active" {
		return store.Run{}, fmt.Errorf("%w: helper is %s", ErrNotRunnable, h.Status)
	}
	version := req.VersionID
	if version == "" && h.CurrentVersionID != nil {
		version = *h.CurrentVersionID
	}
	if version == "" {
		return store.Run{}, fmt.Errorf("%w: helper has no saved version", ErrNotRunnable)
	}
	r := store.Run{
		ID: req.RunID, WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: h.ID, VersionID: version,
		TriggerType: req.TriggerType, TriggerData: req.TriggerData,
		TestMode: req.TestMode || req.TriggerType == "test",
	}
	err = e.Store.CreateRun(ctx, &r)
	if errors.Is(err, store.ErrConflict) && req.RunID != "" {
		return e.Store.GetRun(ctx, req.RunID)
	}
	return r, err
}

// runPayload is the job payload for kind "run": either an existing run, or
// a scheduled trigger (enqueue_due_triggers, migrations/0006).
type runPayload struct {
	RunID        string         `json:"run_id"`
	HelperID     string         `json:"helper_id"`
	VersionID    string         `json:"version_id"`
	TriggerID    string         `json:"trigger_id"`
	TriggerType  string         `json:"trigger_type"`
	TriggerData  map[string]any `json:"trigger_data"`
	ScheduledFor string         `json:"scheduled_for"`
}

func (p runPayload) runID(j queue.Job) string {
	if p.RunID != "" {
		return p.RunID
	}
	return j.ID // scheduled runs use the job id, so retries find the run
}

// HandleJob is the queue handler for "run" jobs.
func (e *Engine) HandleJob(ctx context.Context, j queue.Job) error {
	var p runPayload
	if err := j.Decode(&p); err != nil {
		return queue.Permanent(fmt.Errorf("bad run payload: %w", err))
	}
	if p.RunID == "" {
		data := p.TriggerData
		if p.ScheduledFor != "" {
			data = map[string]any{"scheduled_for": p.ScheduledFor}
		}
		_, err := e.StartRun(ctx, StartRequest{RunID: j.ID, HelperID: p.HelperID, VersionID: p.VersionID,
			TriggerType: p.TriggerType, TriggerData: data})
		switch {
		case errors.Is(err, ErrNotRunnable) || errors.Is(err, store.ErrNotFound):
			e.log().Info("scheduled run skipped", "helper", p.HelperID, "reason", err)
			return nil // e.g. paused since it was queued: nothing to do
		case err != nil:
			return err
		}
	}
	return e.Execute(ctx, p.runID(j))
}

// JobFailed marks the run failed once the queue gives up on it (use as the
// worker's OnFailed for run jobs).
func (e *Engine) JobFailed(ctx context.Context, j queue.Job, cause error) {
	var p runPayload
	_ = j.Decode(&p)
	r, err := e.Store.GetRun(ctx, p.runID(j))
	if err != nil || finished(r.Status) {
		return
	}
	e.finish(ctx, &r, nil, outcome{status: store.RunFailed, err: cause, problem: friendly(cause)})
}

func finished(status string) bool {
	return status == store.RunDone || status == store.RunFailed || status == store.RunStopped
}

// outcome is how a run ends.
type outcome struct {
	status  string // done, failed, stopped
	err     error
	problem string // shown on a Problem card
	nothing bool   // done early: nothing came out
}

// Execute runs (or resumes) a run until it's finished. It returns an error
// only when retrying the job may help (the queue retries with backoff);
// everything else ends the run with a Problem card and returns nil.
func (e *Engine) Execute(ctx context.Context, runID string) error {
	r, err := e.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if finished(r.Status) || r.Status == store.RunWaiting {
		return nil // already done, or waiting for the user (#20)
	}
	h, err := e.Store.GetHelper(ctx, r.HelperID)
	if err != nil {
		return err
	}
	ws, err := e.Store.GetWorkspace(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	v, err := e.Store.GetHelperVersion(ctx, r.VersionID)
	if err != nil {
		return err
	}

	var stack catalog.Stack
	if err := json.Unmarshal(v.Stack, &stack); err != nil {
		e.finish(ctx, &r, nil, outcome{status: store.RunFailed, err: err, problem: "This helper's bricks can't be read. Open it and save it again."})
		return nil
	}
	if errs := catalog.Errors(e.Catalog.Validate(stack, catalog.Options{})); len(errs) > 0 {
		e.finish(ctx, &r, nil, outcome{status: store.RunFailed, err: errors.New(errs[0].Message),
			problem: "This helper needs a fix before it can run: " + errs[0].Message})
		return nil
	}
	for _, sb := range stack.Bricks {
		if _, ok := e.Bricks[sb.Type]; !ok {
			b, _ := e.Catalog.Brick(sb.Type)
			e.finish(ctx, &r, nil, outcome{status: store.RunFailed, err: fmt.Errorf("no executor for %s", sb.Type),
				problem: fmt.Sprintf("“%s” can't run yet.", b.Title.In(ws.Locale))})
			return nil
		}
	}

	rc := &RunContext{
		Run: r, Helper: h, Workspace: ws, TestMode: r.TestMode,
		Limits: limitsFor(stack), Blobs: e.Blobs, Brain: e.Brain, Log: e.log().With("run", r.ID), Now: e.now,
		usage: e.Store, costEUR: float64(r.CostEUR), tokensIn: r.TokensIn, tokensOut: r.TokensOut,
	}
	for k, val := range r.TriggerData {
		if s, ok := val.(string); ok {
			if k == "answer" {
				rc.SetValue("answer", s)
			} else {
				rc.SetValue("trigger."+k, s)
			}
		}
	}

	if r.Status != store.RunRunning || r.StartedAt == nil {
		now := e.now()
		r, err = e.Store.UpdateRun(ctx, r.ID, store.RunPatch{Status: new(store.RunRunning), StartedAt: &now})
		if err != nil {
			return err
		}
		rc.Run = r
	}

	pile, err := e.loadPile(ctx, r)
	if err != nil {
		return err
	}
	steps, err := e.Store.ListSteps(ctx, r.ID)
	if err != nil {
		return err
	}

	for i := r.NextBrickIndex; i < len(stack.Bricks); i++ {
		sb := stack.Bricks[i]
		b, _ := e.Catalog.Brick(sb.Type)

		// Safety stop: always on.
		if len(pile) > rc.Limits.MaxCards {
			e.finish(ctx, &r, rc, outcome{status: store.RunStopped,
				err:     fmt.Errorf("%w: %d cards, max %d", ErrLimit, len(pile), rc.Limits.MaxCards),
				problem: fmt.Sprintf("Stopped before “%s”: %d cards is more than the limit of %d.", b.Title.In(ws.Locale), len(pile), rc.Limits.MaxCards)})
			return nil
		}
		if rc.Limits.MaxCostEUR > 0 && rc.Cost() >= rc.Limits.MaxCostEUR {
			e.finish(ctx, &r, rc, outcome{status: store.RunStopped, err: ErrLimit,
				problem: fmt.Sprintf("Stopped: this run reached its cost limit of €%.2f.", rc.Limits.MaxCostEUR)})
			return nil
		}

		out, stepErr := e.runStep(ctx, rc, &r, steps, i, sb, b, pile)
		if stepErr != nil {
			var se *stepError
			if errors.As(stepErr, &se) && se.final {
				e.finish(ctx, &r, rc, se.outcome)
				return nil
			}
			return stepErr // retry the job
		}
		pile = out

		next := i + 1
		tokIn, tokOut := rc.Tokens()
		r, err = e.Store.UpdateRun(ctx, r.ID, store.RunPatch{NextBrickIndex: &next, CostEUR: new(store.Num(rc.Cost())),
			TokensIn: &tokIn, TokensOut: &tokOut})
		if err != nil {
			return err
		}
		if len(pile) == 0 && b.ShapeFor(b.Config(sb.Config)) != catalog.ShapeStart && next < len(stack.Bricks) {
			e.finish(ctx, &r, rc, outcome{status: store.RunDone, nothing: true})
			return nil
		}
	}
	e.finish(ctx, &r, rc, outcome{status: store.RunDone})
	return nil
}

// stepError carries how a failed step ends the run.
type stepError struct {
	final   bool // end the run now (no retry)
	outcome outcome
	err     error
}

func (s *stepError) Error() string { return s.err.Error() }
func (s *stepError) Unwrap() error { return s.err }

// runStep runs one brick: step row, brick call, shape check, test-mode cap,
// cards saved, step finished.
func (e *Engine) runStep(ctx context.Context, rc *RunContext, r *store.Run, steps []store.Step, i int,
	sb catalog.StackBrick, b *catalog.Brick, in []Card) ([]Card, error) {
	attempt := 1
	for _, s := range steps {
		if s.BrickID == sb.ID && s.Attempt >= attempt {
			attempt = s.Attempt + 1
		}
	}
	step := store.Step{WorkspaceID: r.WorkspaceID, UserID: r.UserID, RunID: r.ID, BrickID: sb.ID,
		BrickType: sb.Type, Position: i, Attempt: attempt, CardsIn: len(in)}
	if err := e.Store.CreateStep(ctx, &step); err != nil {
		return nil, err
	}
	rc.StepID, rc.BrickID, rc.Brick = step.ID, sb.ID, b
	costBefore := rc.Cost()

	cfg := Config(b.Config(sb.Config))
	shape := b.ShapeFor(cfg)
	out, err := e.Bricks[sb.Type].Run(ctx, rc, clonePile(in), cfg)
	if err == nil {
		out, err = CheckShape(shape, in, out)
	}
	if err == nil && rc.TestMode && len(out) > TestModeMaxCards {
		out = out[:TestModeMaxCards]
	}
	if err == nil {
		err = e.saveCards(ctx, rc, step, out)
	}

	done := e.now()
	cost := store.Num(rc.Cost() - costBefore)
	if err != nil {
		e.log().Warn("brick failed", "run", r.ID, "brick", sb.ID, "type", sb.Type, "attempt", attempt, "err", err)
		_, _ = e.Store.UpdateStep(ctx, step.ID, store.StepPatch{Status: new("failed"), FinishedAt: &done, CostEUR: &cost,
			Log: []map[string]any{{"level": "error", "msg": err.Error()}}})
		title := b.Title.In(rc.Workspace.Locale)
		switch {
		case errors.Is(err, llm.ErrAllowance):
			msg := "Your brain allowance for this month is used up. Upgrade your plan, wait until next month, or use your own AI."
			if rc.Workspace.Locale == "de" {
				msg = "Dein Brain-Kontingent für diesen Monat ist aufgebraucht. Wechsle den Tarif, warte bis nächsten Monat oder nutze deine eigene KI."
			}
			return nil, &stepError{final: true, err: err, outcome: outcome{status: store.RunStopped, err: err, problem: msg}}
		case errors.Is(err, ErrLimit):
			return nil, &stepError{final: true, err: err, outcome: outcome{status: store.RunStopped, err: err,
				problem: fmt.Sprintf("Stopped at “%s”: %s.", title, limitText(err))}}
		case errors.Is(err, queue.ErrPermanent), errors.Is(err, errShape), errors.Is(err, ErrMissingValue),
			errors.Is(err, llm.ErrBadOutput), errors.Is(err, llm.ErrNotConfigured):
			return nil, &stepError{final: true, err: err, outcome: outcome{status: store.RunFailed, err: err,
				problem: fmt.Sprintf("“%s” didn't work: %s", title, friendly(err))}}
		}
		return nil, err
	}
	_, err = e.Store.UpdateStep(ctx, step.ID, store.StepPatch{Status: new("done"), CardsOut: new(len(out)),
		FinishedAt: &done, CostEUR: &cost})
	return out, err
}

var errShape = errors.New("brick returned the wrong kind of pile")

// CheckShape enforces a catalog shape on a brick's output and fills in ids
// and parents of new cards. The engine calls it after every brick; brick
// contract tests use it too.
func CheckShape(shape string, in, out []Card) ([]Card, error) {
	inIDs := make(map[string]bool, len(in))
	for _, c := range in {
		inIDs[c.ID] = true
	}
	bad := func(format string, a ...any) ([]Card, error) {
		return nil, fmt.Errorf("%w (%s): %s", errShape, shape, fmt.Sprintf(format, a...))
	}
	switch shape {
	case catalog.ShapeStart, catalog.ShapeCombine:
		if len(out) > 1 {
			return bad("%d cards, at most 1", len(out))
		}
	case catalog.ShapeEnrich, catalog.ShapeAct:
		if len(out) != len(in) {
			return bad("%d cards in, %d out", len(in), len(out))
		}
		for i := range out {
			if out[i].ID != in[i].ID {
				return bad("card %d changed identity", i)
			}
		}
	case catalog.ShapeFilter:
		seen := map[string]bool{}
		for _, c := range out {
			if !inIDs[c.ID] || seen[c.ID] {
				return bad("card %q is new or repeated", c.ID)
			}
			seen[c.ID] = true
		}
	}
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = store.NewID()
		}
		if out[i].Parent == "" && len(in) == 1 && (shape == catalog.ShapeExpand || shape == catalog.ShapeCombine) {
			out[i].Parent = in[0].ID
		}
	}
	return out, nil
}

func clonePile(in []Card) []Card {
	out := make([]Card, len(in))
	for i, c := range in {
		out[i] = c.Clone()
	}
	return out
}

// limitsFor merges stack limits and Safety stop bricks (the strictest wins).
func limitsFor(st catalog.Stack) Limits {
	l := DefaultLimits
	if st.Limits != nil {
		if st.Limits.MaxCards != nil {
			l.MaxCards = *st.Limits.MaxCards
		}
		if st.Limits.MaxExternalEmails != nil {
			l.MaxExternalEmails = *st.Limits.MaxExternalEmails
		}
		if st.Limits.MaxCostEUR != nil {
			l.MaxCostEUR = *st.Limits.MaxCostEUR
		}
	}
	for _, sb := range st.Bricks {
		if sb.Type != "ask.stop" {
			continue
		}
		c := Config(sb.Config)
		if n := c.Int("max_cards"); n > 0 {
			l.MaxCards = min(l.MaxCards, n)
		}
		if _, ok := c["max_emails"]; ok {
			l.MaxExternalEmails = min(l.MaxExternalEmails, c.Int("max_emails"))
		}
		if f := c.Float("max_cost_eur"); f > 0 {
			l.MaxCostEUR = min(l.MaxCostEUR, f)
		}
	}
	return l
}

// ---- piles in the store ----

func (e *Engine) saveCards(ctx context.Context, rc *RunContext, step store.Step, pile []Card) error {
	rows := make([]store.Card, len(pile))
	for i, c := range pile {
		data, err := e.offload(ctx, rc, c)
		if err != nil {
			return err
		}
		row := store.Card{WorkspaceID: step.WorkspaceID, UserID: step.UserID, RunID: step.RunID, StepID: step.ID,
			CardID: c.ID, Position: i, Kind: cmpOr(c.Kind, "item"), Data: data, Trust: c.Trust,
			Kept: c.Kept, Reason: nilIfEmpty(c.Reason)}
		if c.Source != "" {
			row.SourceURL = &c.Source
		}
		if c.Parent != "" {
			row.ParentID = &c.Parent
		}
		if c.Score != nil {
			row.Score = new(store.Num(*c.Score))
		}
		rows[i] = row
	}
	return e.Store.InsertCards(ctx, rows)
}

// offload moves large text fields to Blobs; the card keeps {"$blob": path}.
func (e *Engine) offload(ctx context.Context, rc *RunContext, c Card) (map[string]any, error) {
	data := make(map[string]any, len(c.Fields))
	for k, v := range c.Fields {
		s, isText := v.(string)
		if e.Blobs == nil || !isText || len(s) <= offloadBytes {
			data[k] = v
			continue
		}
		path := fmt.Sprintf("runs/%s/%s/%s/%s.txt", rc.Run.ID, rc.StepID, c.ID, k)
		if err := e.Blobs.Put(ctx, path, []byte(s)); err != nil {
			return nil, fmt.Errorf("store %s of card %s: %w", k, c.ID, err)
		}
		data[k] = map[string]any{"$blob": path, "bytes": float64(len(s))}
	}
	return data, nil
}

// loadPile returns the pile of the last finished step before the run's next
// brick (empty at the start).
func (e *Engine) loadPile(ctx context.Context, r store.Run) ([]Card, error) {
	if r.NextBrickIndex == 0 {
		return nil, nil
	}
	steps, err := e.Store.ListSteps(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	var last *store.Step
	for i := range steps {
		s := &steps[i]
		if s.Position == r.NextBrickIndex-1 && s.Status == "done" && (last == nil || s.Attempt > last.Attempt) {
			last = s
		}
	}
	if last == nil {
		return nil, fmt.Errorf("run %s: no finished step before brick %d", r.ID, r.NextBrickIndex)
	}
	rows, err := e.Store.ListCards(ctx, last.ID)
	if err != nil {
		return nil, err
	}
	pile := make([]Card, len(rows))
	for i, row := range rows {
		c := Card{ID: row.CardID, Kind: row.Kind, Fields: row.Data, Trust: row.Trust, Kept: row.Kept}
		if row.SourceURL != nil {
			c.Source = *row.SourceURL
		}
		if row.ParentID != nil {
			c.Parent = *row.ParentID
		}
		if row.Score != nil {
			c.Score = new(float64(*row.Score))
		}
		if row.Reason != nil {
			c.Reason = *row.Reason
		}
		for k, v := range c.Fields {
			ref, ok := v.(map[string]any)
			path, isBlob := ref["$blob"].(string)
			if !ok || !isBlob || e.Blobs == nil {
				continue
			}
			b, err := e.Blobs.Get(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("load %s of card %s: %w", k, c.ID, err)
			}
			c.Fields[k] = string(b)
		}
		pile[i] = c
	}
	return pile, nil
}

// ---- finishing ----

// finish marks the run finished and puts a Result or Problem card in the Feed.
func (e *Engine) finish(ctx context.Context, r *store.Run, rc *RunContext, o outcome) {
	now := e.now()
	patch := store.RunPatch{Status: &o.status, FinishedAt: &now}
	if o.err != nil {
		patch.Error = store.Set(truncate(o.err.Error(), 2000))
	}
	if rc != nil {
		patch.CostEUR = new(store.Num(rc.Cost()))
	}
	if updated, err := e.Store.UpdateRun(ctx, r.ID, patch); err != nil {
		e.log().Error("finish run", "run", r.ID, "err", err)
	} else {
		*r = updated
	}

	h, _ := e.Store.GetHelper(ctx, r.HelperID)
	ws, _ := e.Store.GetWorkspace(ctx, r.WorkspaceID)
	de := ws.Locale == "de"
	card := store.FeedCard{WorkspaceID: r.WorkspaceID, UserID: r.UserID, HelperID: &r.HelperID, RunID: &r.ID,
		Payload: map[string]any{"status": o.status, "test": r.TestMode}}
	switch {
	case o.status == store.RunDone && o.nothing:
		card.Type, card.Title = "result", pick(de, "Nothing new this time", "Diesmal nichts Neues")
	case o.status == store.RunDone:
		card.Type, card.Title = "result", pick(de, "Done", "Fertig")
		if rc != nil {
			if body := e.resultBody(ctx, r); body != "" {
				card.Body = &body
			}
		}
	default:
		card.Type, card.Title = "problem", pick(de, "Something went wrong", "Etwas ist schiefgelaufen")
		if o.status == store.RunStopped {
			card.Title = pick(de, "Stopped for safety", "Zur Sicherheit gestoppt")
		}
		msg := cmpOr(o.problem, "Please try again later.")
		card.Body = &msg
	}
	if h.Name != "" {
		card.Title = h.Name + ": " + card.Title
	}
	if err := e.Store.CreateFeedCard(ctx, &card); err != nil {
		e.log().Error("feed card", "run", r.ID, "err", err)
	}
	e.log().Info("run finished", "run", r.ID, "status", o.status, "err", o.err)
}

// resultBody summarises the last pile: a summary text, or a count.
func (e *Engine) resultBody(ctx context.Context, r *store.Run) string {
	steps, err := e.Store.ListSteps(ctx, r.ID)
	if err != nil || len(steps) == 0 {
		return ""
	}
	last := steps[len(steps)-1]
	for _, s := range slices.Backward(steps) {
		if s.Status == "done" {
			last = s
			break
		}
	}
	cards, err := e.Store.ListCards(ctx, last.ID)
	if err != nil {
		return ""
	}
	if len(cards) == 1 {
		for _, f := range []string{"summary", "body", "title"} {
			if s, ok := cards[0].Data[f].(string); ok && s != "" {
				return truncate(s, 500)
			}
		}
	}
	return fmt.Sprintf("%d cards", len(cards))
}

func pick(de bool, en, deText string) string {
	if de {
		return deText
	}
	return en
}

// friendly turns an error into a short sentence for a Problem card.
func friendly(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, queue.ErrPermanent.Error()+"\n")
	return truncate(msg, 300)
}

func limitText(err error) string {
	msg := err.Error()
	if _, after, ok := strings.Cut(msg, ErrLimit.Error()+": "); ok {
		return after
	}
	return msg
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

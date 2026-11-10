package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// Brick executes one brick type. It gets the pile from the brick above and
// returns its own pile, following its catalog shape:
//
//	start   nothing → 0 or 1 card       expand  → new cards
//	enrich  same cards, more fields     filter  → a subset of the cards
//	combine → at most 1 new card        act     → the same cards
//
// The engine checks the shape and fills in IDs of new cards. Return
// queue.Permanent(err) for errors a retry won't fix.
type Brick interface {
	Run(ctx context.Context, rc *RunContext, in []Card, cfg Config) ([]Card, error)
}

// BrickFunc adapts a function to Brick.
type BrickFunc func(ctx context.Context, rc *RunContext, in []Card, cfg Config) ([]Card, error)

func (f BrickFunc) Run(ctx context.Context, rc *RunContext, in []Card, cfg Config) ([]Card, error) {
	return f(ctx, rc, in, cfg)
}

// Blobs stores large card content (full page text, attachments) outside
// the cards table; the card keeps a reference. The Eurobase storage
// implementation comes with Read a webpage (#26).
type Blobs interface {
	Put(ctx context.Context, path string, data []byte) error
	Get(ctx context.Context, path string) ([]byte, error)
}

// Limits for one run. Safety stop is always on: these apply even when the
// stack has no Safety stop brick.
type Limits struct {
	MaxCards          int
	MaxExternalEmails int
	MaxCostEUR        float64
}

// DefaultLimits apply when the stack sets none.
var DefaultLimits = Limits{MaxCards: 50, MaxExternalEmails: 10, MaxCostEUR: 0.5}

// TestModeMaxCards: in a test run, piles are cut to this many cards.
const TestModeMaxCards = 3

// ErrLimit is wrapped by errors that stop a run because a limit was hit.
var ErrLimit = errors.New("limit reached")

// ErrMissingValue is wrapped by errors for a pill without a value ({email}
// on a card that has none, an unset {my city}). The run fails with a Problem
// card saying what's missing; retrying wouldn't help.
var ErrMissingValue = errors.New("missing value")

// RunContext is everything a brick needs that isn't a card.
type RunContext struct {
	Run       store.Run
	Helper    store.Helper
	Workspace store.Workspace
	StepID    string         // the current step (run_steps.id)
	BrickID   string         // the current brick's id in the stack, e.g. "b3"
	Brick     *catalog.Brick // the current brick's catalog entry

	// TestMode: at most 3 cards, and Do bricks send to the user instead of
	// anyone else (each Do brick checks this).
	TestMode bool
	Limits   Limits
	Blobs    Blobs
	Log      *slog.Logger
	Now      func() time.Time

	mu        sync.Mutex
	values    map[string]string // run values: answer, trigger.subject, …
	costEUR   float64           // this run so far (including earlier jobs)
	extEmails int
}

// Profile returns an About me fact ("city", "business", …).
func (rc *RunContext) Profile(key string) (string, bool) {
	v, ok := rc.Workspace.Profile[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

// Value returns a run value such as "answer" or "trigger.subject".
func (rc *RunContext) Value(key string) (string, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	v, ok := rc.values[key]
	return v, ok
}

// SetValue stores a run value for later bricks (e.g. Ask me something's
// answer).
func (rc *RunContext) SetValue(key, v string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.values == nil {
		rc.values = map[string]string{}
	}
	rc.values[key] = v
}

// AddCost records money spent (model, search, …) and fails with ErrLimit
// once the run is over its cost limit. Bricks call it after each paid call.
func (rc *RunContext) AddCost(eur float64) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.costEUR += eur
	if rc.Limits.MaxCostEUR > 0 && rc.costEUR > rc.Limits.MaxCostEUR {
		return fmt.Errorf("%w: this run cost more than €%.2f", ErrLimit, rc.Limits.MaxCostEUR)
	}
	return nil
}

// Cost is what the run has spent so far.
func (rc *RunContext) Cost() float64 {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.costEUR
}

// UseExternalEmail reserves one email to someone other than the user. Do
// bricks call it before each such email; ErrLimit means stop sending.
func (rc *RunContext) UseExternalEmail() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.extEmails >= rc.Limits.MaxExternalEmails {
		return fmt.Errorf("%w: at most %d emails to other people per run", ErrLimit, rc.Limits.MaxExternalEmails)
	}
	rc.extEmails++
	return nil
}

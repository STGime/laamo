// Package store is Laamo's data access layer. The engine, bricks, API and
// workers only see these interfaces.
//
// Implementations:
//   - store/eurobase: the Eurobase REST API (MVP)
//   - store/memory:   in-memory fake for unit tests
//   - direct Postgres (sqlc) later, once Laamo has a dedicated instance
//
// Realtime rule: runs, run_steps and feed_cards are watched live by the app,
// so implementations write them one row at a time through REST (which fires
// realtime events). Card piles go through the insert_cards RPC (no events).
package store

import (
	"context"
	"errors"
)

var (
	// ErrNotFound: no row with that id (or matching the lookup).
	ErrNotFound = errors.New("store: not found")
	// ErrConflict: a unique constraint was hit, e.g. an outbound idempotency
	// key or a list row dedupe key that already exists.
	ErrConflict = errors.New("store: conflict")
	// ErrTooLarge: a single row is over the request size limit. Put large
	// content in storage and keep a reference on the card.
	ErrTooLarge = errors.New("store: row too large")
)

// Store is everything the services need.
type Store interface {
	WorkspaceStore
	HelperStore
	RunStore
	CardStore
	FeedStore
	ListStore
	UsageStore
	OutboundStore
}

// Create* methods fill in a missing ID and copy back server defaults
// (status, created_at, …) into the passed row. Every non-zero field is
// written, created_at included, so build new rows fresh rather than copying
// a stored one.
// Update* methods apply only the non-nil fields of the patch and return the new row.

type WorkspaceStore interface {
	GetWorkspace(ctx context.Context, id string) (Workspace, error)
}

type HelperStore interface {
	GetHelper(ctx context.Context, id string) (Helper, error)
	GetHelperVersion(ctx context.Context, id string) (HelperVersion, error)
}

type RunStore interface {
	CreateRun(ctx context.Context, r *Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, id string, p RunPatch) (Run, error)
	// ListRuns returns newest first.
	ListRuns(ctx context.Context, f RunFilter) ([]Run, error)

	CreateStep(ctx context.Context, s *Step) error
	UpdateStep(ctx context.Context, id string, p StepPatch) (Step, error)
	// ListSteps returns a run's steps by position, then attempt.
	ListSteps(ctx context.Context, runID string) ([]Step, error)
}

type CardStore interface {
	// InsertCards writes a pile. Missing IDs are filled in. The pile is split
	// into chunks under the request limit; each chunk is all-or-nothing.
	InsertCards(ctx context.Context, cards []Card) error
	// ListCards returns a step's pile by position.
	ListCards(ctx context.Context, stepID string) ([]Card, error)
}

type FeedStore interface {
	CreateFeedCard(ctx context.Context, c *FeedCard) error
	GetFeedCard(ctx context.Context, id string) (FeedCard, error)
	UpdateFeedCard(ctx context.Context, id string, p FeedCardPatch) (FeedCard, error)
}

type ListStore interface {
	CreateList(ctx context.Context, l *List) error
	GetListByName(ctx context.Context, workspaceID, name string) (List, error)
	// AddListRow returns ErrConflict when a row with the same dedupe key exists.
	AddListRow(ctx context.Context, r *ListRow) error
	// ListRows returns newest first, at most limit rows (0 = all).
	ListRows(ctx context.Context, listID string, limit int) ([]ListRow, error)
}

type UsageStore interface {
	AddUsage(ctx context.Context, u *UsageEntry) error
}

type OutboundStore interface {
	// CreateOutbound returns ErrConflict when the idempotency key exists:
	// the message was already recorded (and maybe sent) by an earlier attempt.
	CreateOutbound(ctx context.Context, m *OutboundMessage) error
	GetOutboundByKey(ctx context.Context, key string) (OutboundMessage, error)
	UpdateOutbound(ctx context.Context, id string, p OutboundPatch) (OutboundMessage, error)
}

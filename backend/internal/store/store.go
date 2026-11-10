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
	// ErrInvalid: the database refused a value (bad timezone, too many
	// workspaces, …). The error text says why and is safe to show.
	ErrInvalid = errors.New("store: invalid")
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
	BrainStore
	TriggerStore
	PlaceStore
	OutboundStore
}

// Create* methods fill in a missing ID and copy back server defaults
// (status, created_at, …) into the passed row. Every non-zero field is
// written, created_at included, so build new rows fresh rather than copying
// a stored one.
// Update* methods apply only the non-nil fields of the patch and return the new row.

type WorkspaceStore interface {
	GetWorkspace(ctx context.Context, id string) (Workspace, error)
	// GetWorkspaces returns the workspaces with these ids (missing ones are
	// left out), in no particular order.
	GetWorkspaces(ctx context.Context, ids []string) ([]Workspace, error)
	// CreateWorkspace creates a workspace with owner as its owner member and
	// the included brain, in one transaction. ErrInvalid for a bad name,
	// timezone or locale, or when the owner has 5 workspaces.
	CreateWorkspace(ctx context.Context, w NewWorkspace) (Workspace, error)
	UpdateWorkspace(ctx context.Context, id string, p WorkspacePatch) (Workspace, error)
	// Memberships returns the user's memberships, oldest first.
	Memberships(ctx context.Context, userID string) ([]Member, error)
	// Membership returns the user's membership in a workspace, or ErrNotFound.
	Membership(ctx context.Context, workspaceID, userID string) (Member, error)
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

type TriggerStore interface {
	// HelperTriggers returns a helper's triggers, oldest first.
	HelperTriggers(ctx context.Context, helperID string) ([]Trigger, error)
	CreateTrigger(ctx context.Context, t *Trigger) error
	UpdateTrigger(ctx context.Context, id string, p TriggerPatch) (Trigger, error)
	// DeleteTrigger removes a trigger; a missing one is not an error.
	DeleteTrigger(ctx context.Context, id string) error
}

type PlaceStore interface {
	CreatePlace(ctx context.Context, p *Place) error
	GetPlace(ctx context.Context, id string) (Place, error)
}

type BrainStore interface {
	// GetBrain returns a workspace's brain, or ErrNotFound.
	GetBrain(ctx context.Context, workspaceID string) (Brain, error)
	// AddBrainUsage counts credits against the brain (a new month starts a
	// new period) and returns the updated brain.
	AddBrainUsage(ctx context.Context, workspaceID string, credits float64) (Brain, error)
}

type OutboundStore interface {
	// CreateOutbound returns ErrConflict when the idempotency key exists:
	// the message was already recorded (and maybe sent) by an earlier attempt.
	CreateOutbound(ctx context.Context, m *OutboundMessage) error
	GetOutboundByKey(ctx context.Context, key string) (OutboundMessage, error)
	UpdateOutbound(ctx context.Context, id string, p OutboundPatch) (OutboundMessage, error)
}

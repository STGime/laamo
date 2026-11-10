// Package queue runs background jobs (runs, digests, …).
//
// MVP implementation: the jobs table on the Eurobase API (queue/eurobase).
// Later: River on a dedicated Postgres instance. Workers only see Queue.
//
// A claimed job carries the claim (ClaimedBy + Attempts). Extend, Complete,
// Retry and Fail only act while that claim still holds; otherwise they
// return ErrLeaseLost and the job belongs to whoever reclaimed it.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrLeaseLost: the worker no longer holds the job (its lease expired and
	// another worker claimed it, or it was already finished).
	ErrLeaseLost = errors.New("queue: lease lost")
	// ErrPermanent marks a handler error that retrying won't fix.
	ErrPermanent = errors.New("queue: permanent error")
)

// Permanent wraps err so the worker fails the job instead of retrying it.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrPermanent, err)
}

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// LeaseDuration is the lease claim_jobs gives (migrations/0006).
const LeaseDuration = 5 * time.Minute

type Job struct {
	ID          string          `json:"id"`
	WorkspaceID *string         `json:"workspace_id,omitempty"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Status      string          `json:"status,omitzero"`
	RunAfter    time.Time       `json:"run_after,omitzero"`
	Attempts    int             `json:"attempts,omitzero"`
	MaxAttempts int             `json:"max_attempts,omitzero"`
	ClaimedBy   *string         `json:"claimed_by,omitempty"`
	ClaimedAt   *time.Time      `json:"claimed_at,omitempty"`
	LeaseUntil  *time.Time      `json:"lease_until,omitempty"`
	LastError   *string         `json:"last_error,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitzero"`
}

// Decode unmarshals the payload into v.
func (j Job) Decode(v any) error { return json.Unmarshal(j.Payload, v) }

// NewJob is what callers enqueue. Keep payloads to ids: no user content or
// secrets (job rows are visible to every worker and in logs).
type NewJob struct {
	WorkspaceID string    // optional
	Kind        string    // required
	Payload     any       // JSON-encoded; nil = {}
	RunAfter    time.Time // zero = now
	MaxAttempts int       // 0 = default (5)
}

type Queue interface {
	// Enqueue adds a job. On Eurobase it is a REST insert, which also wakes
	// idle workers over realtime.
	Enqueue(ctx context.Context, j NewJob) (Job, error)
	// Claim takes up to n due jobs for worker, each with a 5 minute lease.
	// Jobs whose lease expired are taken over (crash recovery).
	Claim(ctx context.Context, worker string, n int) ([]Job, error)
	// Extend pushes the lease of a claimed job to now + d.
	Extend(ctx context.Context, j Job, d time.Duration) error
	// Complete marks a claimed job done.
	Complete(ctx context.Context, j Job) error
	// Retry puts a claimed job back with a delay, or fails it when it has no
	// attempts left (failed = true).
	Retry(ctx context.Context, j Job, cause error, after time.Duration) (failed bool, err error)
	// Fail marks a claimed job failed without retrying.
	Fail(ctx context.Context, j Job, cause error) error
}

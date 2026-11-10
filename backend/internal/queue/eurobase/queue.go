// Package eurobase implements queue.Queue on the Eurobase jobs table.
//
//   - Enqueue: REST insert into jobs (fires a realtime wake-up event)
//   - Claim:   claim_jobs RPC (FOR UPDATE SKIP LOCKED, 5 minute lease)
//   - Extend:  extend_lease RPC; Complete/Retry/Fail: finish_job RPC.
//     Both check the claim, so a worker that lost its lease can't touch the
//     job (REST PATCH can't do that: it updates by id only).
package eurobase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

type Queue struct{ c *eb.Client }

var _ queue.Queue = (*Queue)(nil)

func New(c *eb.Client) *Queue { return &Queue{c: c} }

type insertRow struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id,omitzero"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	RunAfter    time.Time       `json:"run_after,omitzero"`
	MaxAttempts int             `json:"max_attempts,omitzero"`
}

func (q *Queue) Enqueue(ctx context.Context, nj queue.NewJob) (queue.Job, error) {
	if nj.Kind == "" {
		return queue.Job{}, fmt.Errorf("queue: job kind is required")
	}
	row := insertRow{ID: store.NewID(), WorkspaceID: nj.WorkspaceID, Kind: nj.Kind,
		Payload: json.RawMessage(`{}`), RunAfter: nj.RunAfter, MaxAttempts: nj.MaxAttempts}
	if nj.Payload != nil {
		b, err := json.Marshal(nj.Payload)
		if err != nil {
			return queue.Job{}, fmt.Errorf("queue: encode payload: %w", err)
		}
		row.Payload = b
	}
	var j queue.Job
	if err := q.c.Insert(ctx, "jobs", row.ID, row, &j); err != nil {
		return queue.Job{}, fmt.Errorf("queue: enqueue %s: %w", nj.Kind, err)
	}
	return j, nil
}

func (q *Queue) Claim(ctx context.Context, worker string, n int) ([]queue.Job, error) {
	var jobs []queue.Job
	err := q.c.RPC(ctx, "claim_jobs", map[string]any{"worker": worker, "n": n}, &jobs)
	if err != nil {
		// An uncertain claim may have claimed jobs we never saw. They come
		// back when their lease expires; nothing else to do.
		return nil, fmt.Errorf("queue: claim: %w", err)
	}
	return jobs, nil
}

type applied struct {
	Applied bool    `json:"applied"`
	Status  *string `json:"status"`
}

func claim(j queue.Job) (string, error) {
	if j.ClaimedBy == nil || *j.ClaimedBy == "" || j.Attempts < 1 {
		return "", fmt.Errorf("queue: job %s was not claimed: %w", j.ID, queue.ErrLeaseLost)
	}
	return *j.ClaimedBy, nil
}

func (q *Queue) Extend(ctx context.Context, j queue.Job, d time.Duration) error {
	worker, err := claim(j)
	if err != nil {
		return err
	}
	secs := min(max(int(math.Ceil(d.Seconds())), 1), 3600)
	var res applied
	if err := q.c.RPC(ctx, "extend_lease", map[string]any{
		"job_id": j.ID, "worker": worker, "attempt": j.Attempts, "seconds": secs,
	}, &res); err != nil {
		return fmt.Errorf("queue: extend %s: %w", j.ID, err)
	}
	if !res.Applied {
		return fmt.Errorf("queue: extend %s: %w", j.ID, queue.ErrLeaseLost)
	}
	return nil
}

// finish calls finish_job. If the answer is lost (ErrUncertain), the job is
// read back: still ours and running → the call didn't land, send it again;
// same attempt but no longer running → it landed.
func (q *Queue) finish(ctx context.Context, j queue.Job, outcome string, cause error, after time.Duration) (string, error) {
	worker, err := claim(j)
	if err != nil {
		return "", err
	}
	args := map[string]any{"job_id": j.ID, "worker": worker, "attempt": j.Attempts, "outcome": outcome}
	if cause != nil {
		args["error"] = cause.Error()
	}
	if outcome == "retry" {
		args["retry_in"] = min(max(int(math.Ceil(after.Seconds())), 0), 86400)
	}

	const tries = 2
	for try := 1; ; try++ {
		var res applied
		err = q.c.RPC(ctx, "finish_job", args, &res)
		if err == nil {
			if !res.Applied || res.Status == nil {
				return "", fmt.Errorf("queue: finish %s: %w", j.ID, queue.ErrLeaseLost)
			}
			return *res.Status, nil
		}
		if !errors.Is(err, eb.ErrUncertain) {
			return "", fmt.Errorf("queue: finish %s: %w", j.ID, err)
		}
		var cur queue.Job
		if gerr := q.c.Get(ctx, "jobs", j.ID, &cur); gerr != nil {
			return "", fmt.Errorf("queue: finish %s: %w (check failed: %w)", j.ID, err, gerr)
		}
		switch {
		case cur.Attempts != j.Attempts:
			return "", fmt.Errorf("queue: finish %s: %w", j.ID, queue.ErrLeaseLost)
		case cur.Status != queue.StatusRunning:
			return cur.Status, nil // it landed
		case try >= tries:
			return "", fmt.Errorf("queue: finish %s: %w", j.ID, err)
		}
	}
}

func (q *Queue) Complete(ctx context.Context, j queue.Job) error {
	_, err := q.finish(ctx, j, "done", nil, 0)
	return err
}

func (q *Queue) Retry(ctx context.Context, j queue.Job, cause error, after time.Duration) (bool, error) {
	status, err := q.finish(ctx, j, "retry", cause, after)
	return status == queue.StatusFailed, err
}

func (q *Queue) Fail(ctx context.Context, j queue.Job, cause error) error {
	_, err := q.finish(ctx, j, "failed", cause, 0)
	return err
}

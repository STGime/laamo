// Package memory is an in-memory queue.Queue for tests. It follows the
// semantics of claim_jobs, extend_lease and finish_job (migrations/0006,
// 0007): leases, takeover of expired leases, attempt limits and claim checks.
package memory

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

type Queue struct {
	mu   sync.Mutex
	now  func() time.Time
	jobs map[string]*queue.Job
	// Enqueued receives a value per Enqueue (non-blocking), like the
	// realtime wake-up signal.
	Enqueued chan struct{}
}

var _ queue.Queue = (*Queue)(nil)

func New() *Queue {
	return &Queue{now: time.Now, jobs: map[string]*queue.Job{}, Enqueued: make(chan struct{}, 1)}
}

// SetClock replaces time.Now (tests that move time forward).
func (q *Queue) SetClock(now func() time.Time) { q.now = now }

// Jobs returns copies of all jobs, oldest first.
func (q *Queue) Jobs() []queue.Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]queue.Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		out = append(out, *j)
	}
	slices.SortFunc(out, func(a, b queue.Job) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func (q *Queue) Enqueue(_ context.Context, nj queue.NewJob) (queue.Job, error) {
	if nj.Kind == "" {
		return queue.Job{}, fmt.Errorf("queue: job kind is required")
	}
	payload := json.RawMessage(`{}`)
	if nj.Payload != nil {
		b, err := json.Marshal(nj.Payload)
		if err != nil {
			return queue.Job{}, fmt.Errorf("queue: encode payload: %w", err)
		}
		payload = b
	}
	q.mu.Lock()
	now := q.now()
	j := &queue.Job{
		ID: store.NewID(), Kind: nj.Kind, Payload: payload, Status: queue.StatusQueued,
		RunAfter: cmp.Or(nj.RunAfter, now), MaxAttempts: cmp.Or(nj.MaxAttempts, 5), CreatedAt: now,
	}
	if nj.WorkspaceID != "" {
		j.WorkspaceID = &nj.WorkspaceID
	}
	q.jobs[j.ID] = j
	out := *j
	q.mu.Unlock()

	select {
	case q.Enqueued <- struct{}{}:
	default:
	}
	return out, nil
}

func (q *Queue) Claim(_ context.Context, worker string, n int) ([]queue.Job, error) {
	if n < 1 || n > 100 {
		return nil, fmt.Errorf("queue: n must be between 1 and 100")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()

	var due []*queue.Job
	for _, j := range q.jobs {
		expired := j.Status == queue.StatusRunning && j.LeaseUntil != nil && j.LeaseUntil.Before(now)
		queued := j.Status == queue.StatusQueued && !j.RunAfter.After(now)
		switch {
		case (expired || queued) && j.Attempts >= j.MaxAttempts:
			j.Status = queue.StatusFailed
			j.LeaseUntil = nil
			msg := "no attempts left"
			if expired {
				msg = "lease expired after final attempt"
			}
			appendError(j, msg)
		case expired || queued:
			due = append(due, j)
		}
	}
	slices.SortFunc(due, func(a, b *queue.Job) int {
		return cmp.Or(a.RunAfter.Compare(b.RunAfter), cmp.Compare(a.ID, b.ID))
	})
	out := []queue.Job{}
	for _, j := range due[:min(n, len(due))] {
		lease := now.Add(queue.LeaseDuration)
		j.Status = queue.StatusRunning
		j.ClaimedBy = &worker
		j.ClaimedAt = &now
		j.LeaseUntil = &lease
		j.Attempts++
		out = append(out, *j)
	}
	return out, nil
}

// held returns the job if j's claim still holds (caller holds q.mu).
func (q *Queue) held(j queue.Job) (*queue.Job, error) {
	cur, ok := q.jobs[j.ID]
	if !ok || cur.Status != queue.StatusRunning || cur.ClaimedBy == nil || j.ClaimedBy == nil ||
		*cur.ClaimedBy != *j.ClaimedBy || cur.Attempts != j.Attempts {
		return nil, queue.ErrLeaseLost
	}
	return cur, nil
}

func (q *Queue) Extend(_ context.Context, j queue.Job, d time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, err := q.held(j)
	if err != nil {
		return err
	}
	until := q.now().Add(d)
	cur.LeaseUntil = &until
	return nil
}

func (q *Queue) Complete(_ context.Context, j queue.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, err := q.held(j)
	if err != nil {
		return err
	}
	cur.Status = queue.StatusDone
	cur.LeaseUntil = nil
	return nil
}

func (q *Queue) Retry(_ context.Context, j queue.Job, cause error, after time.Duration) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, err := q.held(j)
	if err != nil {
		return false, err
	}
	cur.LeaseUntil = nil
	if cause != nil {
		appendError(cur, cause.Error())
	}
	if cur.Attempts >= cur.MaxAttempts {
		cur.Status = queue.StatusFailed
		return true, nil
	}
	cur.Status = queue.StatusQueued
	cur.RunAfter = q.now().Add(after)
	return false, nil
}

func (q *Queue) Fail(_ context.Context, j queue.Job, cause error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, err := q.held(j)
	if err != nil {
		return err
	}
	cur.Status = queue.StatusFailed
	cur.LeaseUntil = nil
	if cause != nil {
		appendError(cur, cause.Error())
	}
	return nil
}

func appendError(j *queue.Job, msg string) {
	s := msg
	if j.LastError != nil {
		s = *j.LastError + "\n" + msg
	}
	if len(s) > 4000 {
		s = s[len(s)-4000:]
	}
	s = strings.ToValidUTF8(s, "")
	j.LastError = &s
}

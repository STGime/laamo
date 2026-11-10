package queue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/queue/memory"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// clock is a settable clock for the memory queue.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Microsecond)
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func setup(t *testing.T) (*memory.Queue, *clock) {
	t.Helper()
	q := memory.New()
	c := &clock{t: time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)}
	q.SetClock(c.now)
	return q, c
}

// runWorker starts w and returns a stop function that waits for Run to end.
func runWorker(t *testing.T, w *queue.Worker) (stop func()) {
	t.Helper()
	if w.Log == nil {
		w.Log = quiet
	}
	if w.Poll == 0 {
		w.Poll = 5 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = w.Run(ctx)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	return stop
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func status(q *memory.Queue, id string) string {
	for _, j := range q.Jobs() {
		if j.ID == id {
			return j.Status
		}
	}
	return ""
}

func TestWorkerCompletesJobs(t *testing.T) {
	q, _ := setup(t)
	var ran atomic.Int32
	runWorker(t, &queue.Worker{Queue: q, ID: "w1", Handlers: map[string]queue.Handler{
		"run": func(_ context.Context, j queue.Job) error { ran.Add(1); return nil },
	}})
	var ids []string
	for range 25 {
		j, err := q.Enqueue(context.Background(), queue.NewJob{Kind: "run"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	waitFor(t, "all jobs done", func() bool {
		for _, id := range ids {
			if status(q, id) != queue.StatusDone {
				return false
			}
		}
		return true
	})
	if ran.Load() != 25 {
		t.Fatalf("handler ran %d times, want 25", ran.Load())
	}
}

func TestWorkerRetriesThenFails(t *testing.T) {
	q, _ := setup(t)
	var failedJob atomic.Value
	var backoffs []int
	var mu sync.Mutex
	runWorker(t, &queue.Worker{Queue: q, ID: "w1",
		Handlers: map[string]queue.Handler{
			"flaky": func(context.Context, queue.Job) error { return errors.New("upstream 500") },
		},
		Backoff: func(attempt int) time.Duration {
			mu.Lock()
			backoffs = append(backoffs, attempt)
			mu.Unlock()
			return 0
		},
		OnFailed: func(_ context.Context, j queue.Job, cause error) { failedJob.Store(j.ID + ": " + cause.Error()) },
	})
	j, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "flaky", MaxAttempts: 3})
	waitFor(t, "job failed", func() bool { return status(q, j.ID) == queue.StatusFailed })
	waitFor(t, "OnFailed", func() bool { return failedJob.Load() != nil })
	if got := failedJob.Load().(string); got != j.ID+": upstream 500" {
		t.Fatalf("OnFailed got %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(backoffs) != 3 || backoffs[0] != 1 || backoffs[2] != 3 {
		t.Fatalf("backoff called with attempts %v, want [1 2 3]", backoffs)
	}
}

func TestWorkerPermanentError(t *testing.T) {
	q, _ := setup(t)
	var failures atomic.Int32
	runWorker(t, &queue.Worker{Queue: q, ID: "w1",
		Handlers: map[string]queue.Handler{
			"bad": func(context.Context, queue.Job) error { return queue.Permanent(errors.New("stack is invalid")) },
		},
		OnFailed: func(context.Context, queue.Job, error) { failures.Add(1) },
	})
	bad, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "bad"})
	waitFor(t, "failed", func() bool { return status(q, bad.ID) == queue.StatusFailed })
	if j := q.Jobs()[0]; j.Attempts != 1 {
		t.Fatalf("permanent failure should not retry: attempts=%d", j.Attempts)
	}
	waitFor(t, "OnFailed", func() bool { return failures.Load() == 1 })
}

// An unknown kind is handed back (rolling deploys), not failed.
func TestWorkerHandsBackUnknownKind(t *testing.T) {
	q, clk := setup(t)
	runWorker(t, &queue.Worker{Queue: q, ID: "old", Handlers: map[string]queue.Handler{}})
	j, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "new_kind"})
	waitFor(t, "handed back", func() bool {
		got := q.Jobs()[0]
		return got.Status == queue.StatusQueued && got.Attempts == 1
	})
	clk.advance(queue.UnknownKindDelay + time.Second)
	newer, err := q.Claim(context.Background(), "new", 1)
	if err != nil || len(newer) != 1 || newer[0].ID != j.ID {
		t.Fatalf("a newer worker should get the job: %v %d", err, len(newer))
	}
}

func TestWorkerRecoversPanics(t *testing.T) {
	q, _ := setup(t)
	var calls atomic.Int32
	runWorker(t, &queue.Worker{Queue: q, ID: "w1", Backoff: func(int) time.Duration { return 0 },
		Handlers: map[string]queue.Handler{
			"boom": func(context.Context, queue.Job) error {
				if calls.Add(1) == 1 {
					panic("nil map")
				}
				return nil
			},
		}})
	j, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "boom"})
	waitFor(t, "done after panic", func() bool { return status(q, j.ID) == queue.StatusDone })
	for _, got := range q.Jobs() {
		if got.LastError == nil || !strings.Contains(*got.LastError, "handler panic: nil map") {
			t.Fatalf("panic not recorded: %v", got.LastError)
		}
	}
}

// The lease expires while the handler runs and another worker takes the job:
// the first handler's context is cancelled and it must not finish the job.
func TestWorkerStopsOnLostLease(t *testing.T) {
	q, clk := setup(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	runWorker(t, &queue.Worker{Queue: q, ID: "w1", Lease: 30 * time.Millisecond,
		Handlers: map[string]queue.Handler{
			"slow": func(ctx context.Context, _ queue.Job) error {
				close(started)
				<-ctx.Done()
				close(cancelled)
				return nil // pretend success: must not complete the job
			},
		}})
	j, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "slow"})
	<-started
	clk.advance(queue.LeaseDuration + time.Second) // w1 looks dead
	taken, err := q.Claim(context.Background(), "w2", 1)
	if err != nil || len(taken) != 1 {
		t.Fatalf("w2 claim: %v %d", err, len(taken))
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler was not cancelled after losing the lease")
	}
	time.Sleep(20 * time.Millisecond)
	if s := status(q, j.ID); s != queue.StatusRunning {
		t.Fatalf("job should still be running for w2, got %s", s)
	}
	if err := q.Complete(context.Background(), taken[0]); err != nil {
		t.Fatalf("w2 complete: %v", err)
	}
}

// Shutdown cancels the handler and hands the job back right away.
func TestWorkerHandsBackOnShutdown(t *testing.T) {
	q, _ := setup(t)
	started := make(chan struct{})
	stop := runWorker(t, &queue.Worker{Queue: q, ID: "w1", Handlers: map[string]queue.Handler{
		"long": func(ctx context.Context, _ queue.Job) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}})
	j, _ := q.Enqueue(context.Background(), queue.NewJob{Kind: "long"})
	<-started
	stop()
	if s := status(q, j.ID); s != queue.StatusQueued {
		t.Fatalf("job should be back in the queue, got %s", s)
	}
	again, err := q.Claim(context.Background(), "w2", 1)
	if err != nil || len(again) != 1 {
		t.Fatalf("another worker should get it now: %v %d", err, len(again))
	}
}

func TestWorkerWakesUpEarly(t *testing.T) {
	q, _ := setup(t)
	done := make(chan struct{}, 1)
	runWorker(t, &queue.Worker{Queue: q, ID: "w1", Poll: time.Hour, Wake: q.Enqueued,
		Handlers: map[string]queue.Handler{
			"run": func(context.Context, queue.Job) error { done <- struct{}{}; return nil },
		}})
	time.Sleep(20 * time.Millisecond) // worker is now idle, waiting up to an hour
	if _, err := q.Enqueue(context.Background(), queue.NewJob{Kind: "run"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("wake-up signal did not trigger a claim")
	}
}

func TestDefaultBackoff(t *testing.T) {
	for attempt, want := range map[int]time.Duration{1: 30 * time.Second, 2: time.Minute, 3: 2 * time.Minute, 10: 30 * time.Minute} {
		got := queue.DefaultBackoff(attempt)
		if got < want*8/10 || got > want*12/10 {
			t.Fatalf("attempt %d: %v not within 20%% of %v", attempt, got, want)
		}
	}
}

// Package queuetest is a conformance suite for queue.Queue implementations.
package queuetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/queue"
)

// New returns an empty queue and a function that moves its clock forward.
type New func(t *testing.T) (q queue.Queue, advance func(time.Duration))

func Run(t *testing.T, newQueue New) {
	t.Run("EnqueueClaimComplete", func(t *testing.T) { testBasic(t, newQueue) })
	t.Run("ClaimRespectsRunAfterAndLimit", func(t *testing.T) { testDue(t, newQueue) })
	t.Run("RetryAndFail", func(t *testing.T) { testRetry(t, newQueue) })
	t.Run("LeaseTakeover", func(t *testing.T) { testTakeover(t, newQueue) })
	t.Run("Extend", func(t *testing.T) { testExtend(t, newQueue) })
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func claimOne(t *testing.T, q queue.Queue, worker string) queue.Job {
	t.Helper()
	jobs, err := q.Claim(context.Background(), worker, 1)
	must(t, err)
	if len(jobs) != 1 {
		t.Fatalf("%s: want 1 job, got %d", worker, len(jobs))
	}
	return jobs[0]
}

func claimNone(t *testing.T, q queue.Queue, worker string) {
	t.Helper()
	jobs, err := q.Claim(context.Background(), worker, 10)
	must(t, err)
	if len(jobs) != 0 {
		t.Fatalf("%s: want no jobs, got %d", worker, len(jobs))
	}
}

type runPayload struct {
	RunID string `json:"run_id"`
}

func testBasic(t *testing.T, newQueue New) {
	q, _ := newQueue(t)
	ctx := context.Background()

	j, err := q.Enqueue(ctx, queue.NewJob{Kind: "run", WorkspaceID: "aaaaaaaa-1111-0000-0000-000000000001",
		Payload: runPayload{RunID: "r1"}})
	must(t, err)
	if j.ID == "" || j.Status != queue.StatusQueued || j.MaxAttempts != 5 {
		t.Fatalf("enqueued job: %+v", j)
	}
	if _, err := q.Enqueue(ctx, queue.NewJob{}); err == nil {
		t.Fatal("job without kind accepted")
	}

	c := claimOne(t, q, "w1")
	if c.ID != j.ID || c.Status != queue.StatusRunning || c.Attempts != 1 || c.ClaimedBy == nil || *c.ClaimedBy != "w1" {
		t.Fatalf("claimed job: %+v", c)
	}
	if c.LeaseUntil == nil {
		t.Fatal("claimed job has no lease")
	}
	var p runPayload
	must(t, c.Decode(&p))
	if p.RunID != "r1" {
		t.Fatalf("payload: %+v", p)
	}
	claimNone(t, q, "w2")

	must(t, q.Complete(ctx, c))
	if err := q.Complete(ctx, c); !errors.Is(err, queue.ErrLeaseLost) {
		t.Fatalf("second complete: want ErrLeaseLost, got %v", err)
	}
	claimNone(t, q, "w2")

	if _, err := q.Claim(ctx, "w1", 0); err == nil {
		t.Fatal("n=0 accepted")
	}
	if err := q.Complete(ctx, j); !errors.Is(err, queue.ErrLeaseLost) {
		t.Fatalf("complete of an unclaimed job: want ErrLeaseLost, got %v", err)
	}
}

func testDue(t *testing.T, newQueue New) {
	q, _ := newQueue(t)
	ctx := context.Background()
	_, err := q.Enqueue(ctx, queue.NewJob{Kind: "later", RunAfter: time.Now().Add(100 * 365 * 24 * time.Hour)})
	must(t, err)
	for range 3 {
		_, err := q.Enqueue(ctx, queue.NewJob{Kind: "now"})
		must(t, err)
	}
	jobs, err := q.Claim(ctx, "w1", 2)
	must(t, err)
	if len(jobs) != 2 {
		t.Fatalf("limit: want 2, got %d", len(jobs))
	}
	jobs, err = q.Claim(ctx, "w2", 10)
	must(t, err)
	if len(jobs) != 1 || jobs[0].Kind != "now" {
		t.Fatalf("want the last due job only, got %+v", jobs)
	}
}

func testRetry(t *testing.T, newQueue New) {
	q, advance := newQueue(t)
	ctx := context.Background()
	_, err := q.Enqueue(ctx, queue.NewJob{Kind: "flaky", MaxAttempts: 2})
	must(t, err)

	c := claimOne(t, q, "w1")
	failed, err := q.Retry(ctx, c, errors.New("brave timeout"), time.Minute)
	must(t, err)
	if failed {
		t.Fatal("first retry should not fail the job")
	}
	claimNone(t, q, "w2") // backing off
	advance(61 * time.Second)

	c = claimOne(t, q, "w2")
	if c.Attempts != 2 || c.LastError == nil || *c.LastError != "brave timeout" {
		t.Fatalf("second attempt: %+v", c)
	}
	failed, err = q.Retry(ctx, c, errors.New("brave timeout again"), time.Minute)
	must(t, err)
	if !failed {
		t.Fatal("retry after the last attempt should fail the job")
	}
	advance(2 * time.Minute)
	claimNone(t, q, "w3")

	_, err = q.Enqueue(ctx, queue.NewJob{Kind: "broken"})
	must(t, err)
	c = claimOne(t, q, "w1")
	must(t, q.Fail(ctx, c, errors.New("unknown kind")))
	advance(time.Hour)
	claimNone(t, q, "w1")
}

func testTakeover(t *testing.T, newQueue New) {
	q, advance := newQueue(t)
	ctx := context.Background()
	_, err := q.Enqueue(ctx, queue.NewJob{Kind: "slow", MaxAttempts: 2})
	must(t, err)

	stale := claimOne(t, q, "w1")
	advance(queue.LeaseDuration + time.Second) // w1 crashed or hangs
	fresh := claimOne(t, q, "w2")
	if fresh.ID != stale.ID || fresh.Attempts != 2 {
		t.Fatalf("takeover: %+v", fresh)
	}

	for name, err := range map[string]error{
		"complete": q.Complete(ctx, stale),
		"fail":     q.Fail(ctx, stale, errors.New("x")),
		"extend":   q.Extend(ctx, stale, time.Minute),
	} {
		if !errors.Is(err, queue.ErrLeaseLost) {
			t.Fatalf("stale %s: want ErrLeaseLost, got %v", name, err)
		}
	}
	if _, err := q.Retry(ctx, stale, errors.New("x"), 0); !errors.Is(err, queue.ErrLeaseLost) {
		t.Fatalf("stale retry: want ErrLeaseLost, got %v", err)
	}

	// last attempt's lease expires too → failed, not claimed a third time
	advance(queue.LeaseDuration + time.Second)
	claimNone(t, q, "w3")
	if err := q.Complete(ctx, fresh); !errors.Is(err, queue.ErrLeaseLost) {
		t.Fatalf("complete after expiry and failure: want ErrLeaseLost, got %v", err)
	}
}

func testExtend(t *testing.T, newQueue New) {
	q, advance := newQueue(t)
	ctx := context.Background()
	_, err := q.Enqueue(ctx, queue.NewJob{Kind: "long"})
	must(t, err)

	c := claimOne(t, q, "w1")
	advance(4 * time.Minute)
	must(t, q.Extend(ctx, c, 5*time.Minute))
	advance(4 * time.Minute) // 8 minutes after the claim, within the extended lease
	claimNone(t, q, "w2")
	must(t, q.Complete(ctx, c))
}

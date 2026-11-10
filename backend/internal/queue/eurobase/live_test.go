package eurobase_test

import (
	"cmp"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/queue"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
	"github.com/STGime/laamo/backend/internal/store"
)

// TestLiveQueueRoundTrip runs one ping job through a real Eurobase project:
// realtime wake-up, claim, extend, complete, and a stale finish that must be
// refused. The job row is deleted at the end.
// Run with: task test:live (meant for the dev project).
func TestLiveQueueRoundTrip(t *testing.T) {
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 to run against Eurobase")
	}
	key := os.Getenv("EUROBASE_PRIVATE_KEY")
	if key == "" {
		t.Fatal("EUROBASE_PRIVATE_KEY not set")
	}
	url := cmp.Or(os.Getenv("EUROBASE_URL"), "https://laamo.eurobase.app")
	c := eb.New(url, key)
	q := qeb.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	wake := qeb.Wakeups(ctx, url, key, quietLog())
	time.Sleep(2 * time.Second) // let the subscription settle

	j, err := q.Enqueue(ctx, queue.NewJob{Kind: "ping", Payload: map[string]string{"from": "live-test"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), "jobs", j.ID) })

	select {
	case <-wake:
		t.Log("realtime wake-up received")
	case <-time.After(10 * time.Second):
		t.Error("no realtime wake-up within 10s (polling would still work)")
	}

	worker := "live-test-" + store.NewID()[:8]
	var mine queue.Job
	for mine.ID == "" {
		jobs, err := q.Claim(ctx, worker, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 0 {
			t.Fatal("ping job was not claimable")
		}
		if jobs[0].ID != j.ID {
			// Someone else's job on the dev project: hand it back untouched.
			_, _ = q.Retry(ctx, jobs[0], errors.New("handed back by live test"), 0)
			continue
		}
		mine = jobs[0]
	}

	if err := q.Extend(ctx, mine, 2*time.Minute); err != nil {
		t.Fatalf("extend: %v", err)
	}
	stale := mine
	stale.Attempts++
	if err := q.Complete(ctx, stale); !errors.Is(err, queue.ErrLeaseLost) {
		t.Fatalf("finish with a wrong attempt: want ErrLeaseLost, got %v", err)
	}
	if err := q.Complete(ctx, mine); err != nil {
		t.Fatalf("complete: %v", err)
	}
	var row queue.Job
	if err := c.Get(ctx, "jobs", j.ID, &row); err != nil || row.Status != queue.StatusDone {
		t.Fatalf("job after complete: %+v, %v", row, err)
	}
}

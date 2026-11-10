package eurobase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/eurobase/eurobasetest"
	"github.com/STGime/laamo/backend/internal/queue"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
	"github.com/STGime/laamo/backend/internal/queue/queuetest"
)

func newQueue(t *testing.T) (*qeb.Queue, *eurobasetest.Fake) {
	t.Helper()
	fake := eurobasetest.New(t)
	c := eb.New(fake.URL(), fake.Key, eb.WithSleep(func(context.Context, time.Duration) error { return nil }))
	return qeb.New(c), fake
}

func TestConformance(t *testing.T) {
	queuetest.Run(t, func(t *testing.T) (queue.Queue, func(time.Duration)) {
		q, fake := newQueue(t)
		return q, fake.Advance
	})
}

func TestEnqueueIsARestInsert(t *testing.T) {
	q, fake := newQueue(t)
	j, err := q.Enqueue(context.Background(), queue.NewJob{Kind: "run", Payload: map[string]string{"run_id": "r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if fake.Requests[len(fake.Requests)-1] != "POST /v1/db/jobs" {
		t.Fatalf("enqueue should POST /v1/db/jobs (realtime wake-up), got %v", fake.Requests)
	}
	rows := fake.Rows("jobs")
	if len(rows) != 1 || rows[0]["user_id"] != "00000000-0000-0000-0000-000000000000" || rows[0]["id"] != j.ID {
		t.Fatalf("job row: %v", rows)
	}
	if p, _ := rows[0]["payload"].(map[string]any); p["run_id"] != "r1" {
		t.Fatalf("payload: %v", rows[0]["payload"])
	}
}

// finish_job ran but the answer was lost: Complete reads the job back and
// reports success instead of an error.
func TestCompleteWithLostAnswer(t *testing.T) {
	q, fake := newQueue(t)
	ctx := context.Background()
	if _, err := q.Enqueue(ctx, queue.NewJob{Kind: "run"}); err != nil {
		t.Fatal(err)
	}
	jobs, err := q.Claim(ctx, "w1", 1)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("claim: %v %d", err, len(jobs))
	}
	fake.FailNext(http.MethodPost, "/v1/db/rpc/finish_job", eurobasetest.Fault{Status: 504, After: true})
	if err := q.Complete(ctx, jobs[0]); err != nil {
		t.Fatalf("complete with lost answer: %v", err)
	}
	if s := fake.Rows("jobs")[0]["status"]; s != "done" {
		t.Fatalf("status = %v", s)
	}
}

// finish_job never ran (502 before applying): Complete sends it again.
func TestCompleteNotApplied(t *testing.T) {
	q, fake := newQueue(t)
	ctx := context.Background()
	if _, err := q.Enqueue(ctx, queue.NewJob{Kind: "run"}); err != nil {
		t.Fatal(err)
	}
	jobs, _ := q.Claim(ctx, "w1", 1)
	fake.FailNext(http.MethodPost, "/v1/db/rpc/finish_job", eurobasetest.Fault{Status: 502})
	failed, err := q.Retry(ctx, jobs[0], errors.New("timeout"), 10*time.Second)
	if err != nil || failed {
		t.Fatalf("retry: failed=%v err=%v", failed, err)
	}
	row := fake.Rows("jobs")[0]
	if row["status"] != "queued" || row["last_error"] != "timeout" {
		t.Fatalf("job after retry: %v", row)
	}
}

func TestClaimIsNotRetriedWhenUncertain(t *testing.T) {
	q, fake := newQueue(t)
	fake.FailNext(http.MethodPost, "/v1/db/rpc/claim_jobs", eurobasetest.Fault{Status: 504})
	_, err := q.Claim(context.Background(), "w1", 5)
	if !errors.Is(err, eb.ErrUncertain) {
		t.Fatalf("want ErrUncertain, got %v", err)
	}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

package eurobase_test

import (
	"cmp"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
)

// TestLiveReadOnly checks request and response shapes against a real
// Eurobase project. It only reads random ids, so it writes nothing.
// Run with: task test:live (needs EUROBASE_PRIVATE_KEY in .env).
func TestLiveReadOnly(t *testing.T) {
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 to run against Eurobase")
	}
	key := os.Getenv("EUROBASE_PRIVATE_KEY")
	if key == "" {
		t.Fatal("EUROBASE_PRIVATE_KEY not set")
	}
	url := cmp.Or(os.Getenv("EUROBASE_URL"), "https://laamo.eurobase.app")
	c := eb.New(url, key)
	s := sebs.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := s.GetRun(ctx, store.NewID()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetRun(random): want ErrNotFound, got %v", err)
	}
	runs, err := s.ListRuns(ctx, store.RunFilter{HelperID: store.NewID(), Status: store.RunWaiting, Limit: 5})
	if err != nil || len(runs) != 0 {
		t.Fatalf("ListRuns(random helper): %v, %d rows", err, len(runs))
	}
	if _, err := s.GetListByName(ctx, store.NewID(), "Leads & Co."); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetListByName: want ErrNotFound, got %v", err)
	}
	cards, err := s.ListCards(ctx, store.NewID())
	if err != nil || len(cards) != 0 {
		t.Fatalf("ListCards(random step): %v, %d rows", err, len(cards))
	}
	if _, err := s.GetOutboundByKey(ctx, "live-test:"+store.NewID()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetOutboundByKey: want ErrNotFound, got %v", err)
	}

	// RPC errors carry status and SQLSTATE (Eurobase #829). claim_jobs
	// rejects n=0 before touching any row.
	var apiErr *eb.APIError
	err = c.RPC(ctx, "claim_jobs", map[string]any{"worker": "live-test", "n": 0}, nil)
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Code != "22023" {
		t.Fatalf("RPC error shape: want 400 / 22023, got %v", err)
	}
}

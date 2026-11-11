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
)

// TestLiveObjects stores, reads and deletes one test object in project
// storage. Run with: task test:live.
func TestLiveObjects(t *testing.T) {
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 to run against Eurobase")
	}
	key := os.Getenv("EUROBASE_PRIVATE_KEY")
	if key == "" {
		t.Fatal("EUROBASE_PRIVATE_KEY not set")
	}
	c := eb.New(cmp.Or(os.Getenv("EUROBASE_URL"), "https://laamo.eurobase.app"), key)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	obj := "laamo/live-test/" + store.NewID() + "/page_text.txt"
	text := []byte("Bäckerei Krause – Brot seit 1952\n")
	if err := c.PutObject(ctx, obj, "text/plain; charset=utf-8", text); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.DeleteObject(context.Background(), obj) })
	if err := c.PutObject(ctx, obj, "text/plain; charset=utf-8", text); err != nil {
		t.Fatalf("overwrite (a retried step): %v", err)
	}
	got, err := c.GetObject(ctx, obj)
	if err != nil || string(got) != string(text) {
		t.Fatalf("get: %q, %v", got, err)
	}
	if err := c.DeleteObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetObject(ctx, obj); !errors.Is(err, eb.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

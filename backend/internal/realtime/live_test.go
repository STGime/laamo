package realtime_test

import (
	"cmp"
	"context"
	"os"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/realtime"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
)

// TestLiveFeedEvents checks what the API's event hub relies on: a Feed card
// written through REST arrives as a realtime event that carries its
// workspace_id. Needs LAAMO_LIVE_USER_ID (an end user with a workspace).
func TestLiveFeedEvents(t *testing.T) {
	user := os.Getenv("LAAMO_LIVE_USER_ID")
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || user == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and LAAMO_LIVE_USER_ID to run against Eurobase")
	}
	url := cmp.Or(os.Getenv("EUROBASE_URL"), "https://laamo.eurobase.app")
	key := os.Getenv("EUROBASE_PRIVATE_KEY")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := eb.New(url, key)
	st := sebs.New(c)
	members, err := st.Memberships(ctx, user)
	if err != nil || len(members) == 0 {
		t.Fatalf("memberships: %v (%d)", err, len(members))
	}
	ws := members[0].WorkspaceID

	events := realtime.Subscribe(ctx, url, key, []string{"feed_cards"}, nil)
	time.Sleep(2 * time.Second) // subscribed
	fc := store.FeedCard{WorkspaceID: ws, UserID: user, Type: "result", Title: "Live test", State: "dismissed"}
	if err := st.CreateFeedCard(ctx, &fc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), "feed_cards", fc.ID) })
	for {
		select {
		case ev := <-events:
			if ev.Table() == "feed_cards" && ev.String("id") == fc.ID {
				if ev.Type != "INSERT" || ev.String("workspace_id") != ws || ev.String("state") != "dismissed" {
					t.Fatalf("event %+v", ev)
				}
				t.Logf("got %s for feed card %s in workspace %s", ev.Type, fc.ID, ws)
				return
			}
		case <-ctx.Done():
			t.Fatal("no realtime event for the new Feed card")
		}
	}
}

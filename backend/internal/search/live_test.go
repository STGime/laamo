package search

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveBrave runs one real search (1 of the plan's monthly requests).
// Run with: task test:live (needs BRAVE_SEARCH_API_KEY in .env).
func TestLiveBrave(t *testing.T) {
	key := os.Getenv("BRAVE_SEARCH_API_KEY")
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || key == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and BRAVE_SEARCH_API_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := (&Brave{APIKey: key, Limit: NewLimiter(1)}).Search(ctx, Query{Q: "Bäckerei Leipzig", Count: 3, Freshness: "week", Lang: "de", Country: "DE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].URL == "" || res[0].Title == "" {
		t.Fatalf("results: %+v", res)
	}
	t.Logf("%d results, first: %s (%s)", len(res), res[0].Title, res[0].URL)
}

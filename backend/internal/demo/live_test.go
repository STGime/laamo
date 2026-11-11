package demo

import (
	"context"
	"os"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/web"
)

// TestLiveDemo runs the Morning Briefing demo with live search and AI and
// reports time and cost. task test:live.
func TestLiveDemo(t *testing.T) {
	cfg, _ := config.Load()
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || cfg.LLMAPIKey == "" || cfg.BraveSearchAPIKey == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1, an LLM key and BRAVE_SEARCH_API_KEY")
	}
	p := &search.Brave{APIKey: cfg.BraveSearchAPIKey, Limit: search.NewLimiter(cfg.BraveRPS)}
	s := &Service{Catalog: catalog.Default(), Model: cfg.LLMModel,
		Chat: &llm.Client{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Auth: cfg.LLMAuth, Model: cfg.LLMModel},
		Deps: func(st store.Store) bricks.Deps {
			return bricks.Deps{Search: p, SearchCostEUR: cfg.SearchCostEUR, Store: st, Pages: &web.Fetcher{}}
		}}
	s.init()
	s.Limits.Bits = 8
	var cost float64
	run := s.runner
	s.runner = func(ctx context.Context, in Input) (Result, float64, error) {
		r, c, err := run(ctx, in)
		cost = c
		return r, c, err
	}
	res, err := s.Run(context.Background(), "127.0.0.1", input(s, "morning-briefing", "Burger Restaurant", "Berlin"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%.1fs, €%.4f: %s\n%s\nlinks %d, cards %d", res.Seconds, cost, res.Title, res.Body, len(res.Links), len(res.Cards))
	if res.Body == "" || res.Seconds > 60 {
		t.Fatalf("result %+v", res)
	}
}

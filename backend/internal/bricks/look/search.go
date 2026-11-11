// Package look contains the 🔵 Look bricks: they bring outside content in
// as cards. Everything they read is untrusted.
package look

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/geo"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
)

// Search is the Search the web brick (look.search).
type Search struct {
	Provider search.Provider
	CostEUR  float64 // per search, for the usage ledger and run cost limit
}

func (s Search) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if s.Provider == nil {
		return nil, queue.Permanent(errors.New("web search isn't set up"))
	}
	// A query with card fields ({name}) searches once per card; otherwise once.
	perCard := false
	for _, p := range catalog.Pills(cfg.String("query")) {
		perCard = perCard || p.Kind == "field"
	}
	targets := []*engine.Card{nil}
	if perCard && len(in) > 0 {
		targets = targets[:0]
		for i := range in {
			targets = append(targets, &in[i])
		}
	}

	// Results in the workspace's language, from the country it's in (from
	// the About me city, or the browser's timezone): an English-speaking
	// shop in Berlin wants Berlin news, not American ones.
	city, _ := rc.Profile("city")
	q := search.Query{Count: cfg.Int("count"), Freshness: cfg.String("freshness"), Lang: rc.Workspace.Locale,
		Country: geo.Country(city, rc.Workspace.Timezone)}
	seen := map[string]bool{}
	var out []engine.Card
	for _, card := range targets {
		resolved, err := tmpl.Config(rc, rc.Brick, cfg, card)
		if err != nil {
			return nil, err
		}
		q.Q = strings.TrimSpace(resolved.String("query"))
		results, err := s.Provider.Search(ctx, q)
		if errors.Is(err, search.ErrNotConfigured) || errors.Is(err, search.ErrRejected) {
			return nil, queue.Permanent(err)
		}
		if err != nil {
			return nil, fmt.Errorf("search %q: %w", q.Q, err)
		}
		if err := rc.Record(ctx, store.UsageEntry{Kind: "search", Model: new("brave"), Units: 1, EstCostEUR: store.Num(s.CostEUR)}); err != nil {
			return nil, err
		}
		for _, r := range results {
			if r.URL == "" || seen[r.URL] {
				continue
			}
			seen[r.URL] = true
			c := engine.Card{Kind: "web_result", Source: r.URL}
			c.Set("title", r.Title, engine.TrustUntrusted)
			c.Set("url", r.URL, engine.TrustUntrusted)
			c.Set("snippet", r.Snippet, engine.TrustUntrusted)
			if !r.PublishedAt.IsZero() {
				c.Set("published_at", r.PublishedAt.Format("2006-01-02T15:04:05Z"), engine.TrustUntrusted)
			}
			if card != nil {
				c.Parent = card.ID
			}
			out = append(out, c)
		}
	}
	return out, nil
}

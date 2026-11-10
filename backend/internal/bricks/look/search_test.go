package look_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/look"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/search"
)

type fakeSearch struct {
	queries []search.Query
	err     error
}

func (f *fakeSearch) Search(_ context.Context, q search.Query) ([]search.Result, error) {
	f.queries = append(f.queries, q)
	if f.err != nil {
		return nil, f.err
	}
	var out []search.Result
	for i := range q.Count {
		out = append(out, search.Result{Title: fmt.Sprintf("%s #%d", q.Q, i), URL: fmt.Sprintf("https://example.com/%d", i),
			Snippet: "news", PublishedAt: time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)})
	}
	return out, nil
}

func TestSearch(t *testing.T) {
	fs := &fakeSearch{}
	b := look.Search{Provider: fs, CostEUR: 0.005}
	rc := brickstest.RunContext("look.search")
	rc.Workspace.Locale = "de"
	out, err := brickstest.Contract(t, "look.search", b, rc, nil, map[string]any{"query": "news für {my business} in {my city}", "count": 5, "freshness": "day"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5 || out[0].Kind != "web_result" || out[0].Source != "https://example.com/0" || out[0].Trust["title"] != engine.TrustUntrusted {
		t.Fatalf("cards: %+v", out)
	}
	q := fs.queries[0]
	if q.Q != "news für bakery in Leipzig" || q.Count != 5 || q.Freshness != "day" || q.Lang != "de" || q.Country != "DE" {
		t.Fatalf("query: %+v", q)
	}
	if out[0].Text("published_at") != "2026-11-09T07:00:00Z" {
		t.Fatalf("published_at: %s", out[0].Text("published_at"))
	}
	if rc.Cost() != 0.005 {
		t.Fatalf("cost: %v", rc.Cost())
	}
}

func TestSearchPerCard(t *testing.T) {
	fs := &fakeSearch{}
	in := make([]engine.Card, 2)
	for i, name := range []string{"Bäckerei Maier", "Café Lotte"} {
		in[i] = engine.Card{ID: fmt.Sprintf("c%d", i)}
		in[i].Set("name", name, engine.TrustExtracted)
	}
	out, err := brickstest.Contract(t, "look.search", look.Search{Provider: fs}, nil, in, map[string]any{"query": "{name} reviews", "count": 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.queries) != 2 || fs.queries[1].Q != "Café Lotte reviews" {
		t.Fatalf("queries: %+v", fs.queries)
	}
	// Same URLs from both searches are kept once; each points at its card.
	if len(out) != 5 || out[0].Parent != "c0" {
		t.Fatalf("dedup/parents: %d cards, parent %q", len(out), out[0].Parent)
	}
}

func TestSearchErrors(t *testing.T) {
	_, err := brickstest.Contract(t, "look.search", look.Search{Provider: &fakeSearch{err: search.ErrNotConfigured}}, nil, nil, map[string]any{"query": "x"})
	if !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("not configured must be permanent: %v", err)
	}
	_, err = brickstest.Contract(t, "look.search", look.Search{Provider: &fakeSearch{err: errors.New("timeout")}}, nil, nil, map[string]any{"query": "x"})
	if err == nil || errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("a timeout should be retried: %v", err)
	}
	_, err = brickstest.Contract(t, "look.search", look.Search{Provider: &fakeSearch{}}, nil, nil, map[string]any{"query": "news for {my offer}"})
	if !errors.Is(err, engine.ErrMissingValue) {
		t.Fatalf("missing About me fact: %v", err)
	}
}

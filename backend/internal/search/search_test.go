package search

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func braveServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/brave_baeckerei.json")
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	var lastQuery atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/news/search" && r.Header.Get("X-Subscription-Token") == "brave-test" {
			_, _ = w.Write([]byte(`{"results":[]}`)) // no news: tests use the web fallback
			return
		}
		i := int(n.Add(1)) - 1
		lastQuery.Store(r.URL.RawQuery)
		if r.URL.Path != "/web/search" || r.Header.Get("X-Subscription-Token") != "brave-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if i < len(statuses) {
			w.WriteHeader(statuses[i])
			return
		}
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)
	return srv, &n, &lastQuery
}

func TestBraveSearch(t *testing.T) {
	srv, _, q := braveServer(t)
	b := &Brave{APIKey: "brave-test", BaseURL: srv.URL}
	res, err := b.Search(context.Background(), Query{Q: "bäckerei news leipzig", Count: 3, Freshness: "week", Lang: "de", Country: "DE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 results, got %d", len(res))
	}
	got := q.Load().(string)
	for _, want := range []string{"count=3", "freshness=pw", "search_lang=de", "country=DE", "q=b%C3%A4ckerei+news+leipzig"} {
		if !strings.Contains(got, want) {
			t.Errorf("query %q lacks %q", got, want)
		}
	}
	r := res[0]
	if strings.Contains(r.Snippet, "<strong>") || !strings.HasPrefix(r.Snippet, "Anfang August") {
		t.Errorf("snippet not cleaned: %q", r.Snippet)
	}
	if r.URL == "" || r.Title == "" || !r.PublishedAt.Equal(time.Date(2026, 11, 10, 19, 57, 2, 0, time.UTC)) {
		t.Errorf("result: %+v", r)
	}
}

func TestBraveErrors(t *testing.T) {
	ctx := context.Background()
	srv, n, _ := braveServer(t, http.StatusTooManyRequests)
	if res, err := (&Brave{APIKey: "brave-test", BaseURL: srv.URL}).Search(ctx, Query{Q: "x"}); err != nil || len(res) != 3 || n.Load() != 2 {
		t.Fatalf("429 retried: %v (%d calls)", err, n.Load())
	}
	if _, err := (&Brave{APIKey: "wrong", BaseURL: srv.URL}).Search(ctx, Query{Q: "x"}); !errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad key: %v", err)
	}
	if _, err := (&Brave{APIKey: "brave-test", BaseURL: srv.URL}).Search(ctx, Query{Q: "  "}); err == nil {
		t.Fatal("empty query accepted")
	}
	if _, err := (&Brave{BaseURL: srv.URL}).Search(ctx, Query{Q: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no key: %v", err)
	}
}

func TestLimiterSpacesCalls(t *testing.T) {
	clock := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	var waits []time.Duration
	l := NewLimiter(1)
	l.now = func() time.Time { return clock }
	l.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	for range 3 {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Fatalf("waits %v, want [1s 2s]", waits)
	}
	clock = clock.Add(5 * time.Second) // idle for a while: no wait
	waits = nil
	_ = l.Wait(context.Background())
	if len(waits) != 0 {
		t.Fatalf("after idle: %v", waits)
	}
	if NewLimiter(0) != nil || (*Limiter)(nil).Wait(context.Background()) != nil {
		t.Fatal("no limit: nil limiter, no wait")
	}
}

func TestCache(t *testing.T) {
	srv, n, _ := braveServer(t)
	c := &Cached{Provider: &Brave{APIKey: "brave-test", BaseURL: srv.URL}, Dir: t.TempDir()}
	ctx := context.Background()
	q := Query{Q: "bäckerei", Count: 3}
	a, err := c.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Search(ctx, q)
	if err != nil || n.Load() != 1 || len(b) != len(a) || b[0].URL != a[0].URL || !b[0].PublishedAt.Equal(a[0].PublishedAt) {
		t.Fatalf("second call should be cached: %d calls, %v", n.Load(), err)
	}
	if _, err := c.Search(ctx, Query{Q: "bäckerei", Count: 5}); err != nil || n.Load() != 2 {
		t.Fatalf("different query must miss the cache: %d calls", n.Load())
	}
}

// Fresh searches use news search; with no news they fall back to the web,
// without portal front pages. Other searches are plain web searches.
func TestBraveNewsFirst(t *testing.T) {
	news := `{"results":[{"title":"Burgerladen eröffnet","url":"https://www.rbb24.de/berlin/burger-123.html","description":"Neu in Kreuzberg","meta_url":{"hostname":"www.rbb24.de"}}]}`
	web := `{"web":{"results":[{"title":"B.Z. – Die Stimme Berlins","url":"https://www.bz-berlin.de/","description":"News"},
	{"title":"Burger Test","url":"https://www.tip-berlin.de/essen/burger/","description":"Die besten Burger"}]}}`
	var paths []string
	noNews := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == "/news/search" && noNews:
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/news/search":
			_, _ = w.Write([]byte(news))
		default:
			_, _ = w.Write([]byte(web))
		}
	}))
	defer srv.Close()
	b := &Brave{APIKey: "k", BaseURL: srv.URL}
	ctx := context.Background()
	res, err := b.Search(ctx, Query{Q: "burger berlin", Count: 10, Freshness: "day"})
	if err != nil || len(res) != 1 || res[0].Source != "www.rbb24.de" || !slices.Equal(paths, []string{"/news/search"}) {
		t.Fatalf("news: %+v %v %v", res, err, paths)
	}
	noNews, paths = true, nil
	res, _ = b.Search(ctx, Query{Q: "burger berlin", Count: 10, Freshness: "week"})
	if len(res) != 1 || res[0].URL != "https://www.tip-berlin.de/essen/burger/" || len(paths) != 2 {
		t.Fatalf("fallback: %+v %v", res, paths)
	}
	paths = nil
	res, _ = b.Search(ctx, Query{Q: "bäckerei krause website", Count: 10})
	if len(res) != 2 || !slices.Equal(paths, []string{"/web/search"}) {
		t.Fatalf("not fresh: %+v %v", res, paths)
	}
}

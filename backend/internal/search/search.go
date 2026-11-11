// Package search is the web search behind the Search the web brick. The MVP
// provider is Brave Search; a Provider interface keeps it swappable.
//
// Brave is a US service: only the search terms leave the EU (listed as a
// sub-processor in the DPA). Never put email or list content into a query.
package search

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Query is one web search.
type Query struct {
	Q         string
	Count     int    // 1…20
	Freshness string // day, week, any
	Lang      string // en, de, … ("" = any)
	Country   string // DE, … ("" = any)
}

// Result is one search hit.
type Result struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Snippet     string    `json:"snippet"`
	PublishedAt time.Time `json:"published_at,omitzero"`
	Source      string    `json:"source,omitempty"` // site name, e.g. "MDR"
}

// Provider runs searches.
type Provider interface {
	Search(ctx context.Context, q Query) ([]Result, error)
}

// ErrRejected: the search API refused the request (bad key, bad
// parameters); trying again won't help.
var ErrRejected = errors.New("search: request refused")

var langRe = regexp.MustCompile(`^[a-z]{2}$`)

// ErrNotConfigured: no API key.
var ErrNotConfigured = errors.New("search: no API key configured")

// ---- Brave ----

// Brave calls the Brave Search API.
type Brave struct {
	APIKey  string
	BaseURL string // default https://api.search.brave.com/res/v1
	HTTP    *http.Client
	Limit   *Limiter // shared across calls; nil = no limit
}

type braveItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	PageAge     string `json:"page_age"`
	Profile     struct {
		Name string `json:"name"`
	} `json:"profile"`
	MetaURL struct {
		Hostname string `json:"hostname"`
	} `json:"meta_url"`
}

type braveResponse struct {
	Web struct {
		Results []braveItem `json:"results"`
	} `json:"web"`
}

// cacheVersion changes when result processing changes, so the dev cache
// doesn't serve results shaped by older code.
const cacheVersion = 3

var tagRe = regexp.MustCompile(`<[^>]*>`)

// isHomepage reports whether u is a site's front page (no path): fine when
// looking for a business, noise when looking for news.
func isHomepage(u string) bool {
	p, err := url.Parse(u)
	return err == nil && strings.Trim(p.Path, "/") == "" && p.RawQuery == ""
}

func (b *Brave) Search(ctx context.Context, q Query) ([]Result, error) {
	if b.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(q.Q) == "" {
		return nil, errors.New("search: empty query")
	}
	v := url.Values{"q": {q.Q}, "count": {strconv.Itoa(min(max(q.Count, 1), 20))}, "safesearch": {"moderate"}}
	switch q.Freshness {
	case "day":
		v.Set("freshness", "pd")
	case "week":
		v.Set("freshness", "pw")
	}
	if langRe.MatchString(q.Lang) { // anything else would be refused (422)
		v.Set("search_lang", q.Lang)
	}
	if len(q.Country) == 2 {
		v.Set("country", q.Country)
	}
	// Fresh results come from news search: web results "updated today" are
	// mostly portal front pages. No news: fall back to the web, without
	// front pages.
	fresh := q.Freshness == "day" || q.Freshness == "week"
	if fresh {
		var news struct {
			Results []braveItem `json:"results"`
		}
		if err := b.get(ctx, "/news/search", v, &news); err != nil {
			return nil, err
		}
		if out := results(news.Results, false, q.Count); len(out) > 0 {
			return out, nil
		}
	}
	var web braveResponse
	if err := b.get(ctx, "/web/search", v, &web); err != nil {
		return nil, err
	}
	return results(web.Web.Results, fresh, q.Count), nil
}

func results(items []braveItem, skipHomepages bool, count int) []Result {
	out := make([]Result, 0, len(items))
	seen := map[string]bool{}
	for _, r := range items {
		if r.URL == "" || seen[r.URL] || (skipHomepages && isHomepage(r.URL)) {
			continue
		}
		seen[r.URL] = true
		res := Result{Title: clean(r.Title), URL: r.URL, Snippet: clean(r.Description), Source: cmp.Or(r.Profile.Name, r.MetaURL.Hostname)}
		if t, err := time.Parse("2006-01-02T15:04:05", r.PageAge); err == nil {
			res.PublishedAt = t.UTC()
		}
		out = append(out, res)
	}
	if count > 0 && len(out) > count {
		out = out[:count]
	}
	return out
}

// get calls one Brave endpoint, retrying 429s.
func (b *Brave) get(ctx context.Context, path string, v url.Values, out any) error {
	base := b.BaseURL
	if base == "" {
		base = "https://api.search.brave.com/res/v1"
	}
	for attempt := 0; ; attempt++ {
		if err := b.Limit.Wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+v.Encode(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", b.APIKey)
		hc := b.HTTP
		if hc == nil {
			hc = &http.Client{Timeout: 20 * time.Second}
		}
		resp, err := hc.Do(req) //nolint:gosec // G704: fixed API host from config
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("search: read: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			continue // the limiter spaces the next try
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("%w: Brave HTTP %d: %.200s", ErrRejected, resp.StatusCode, body)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("search: Brave HTTP %d: %.200s", resp.StatusCode, body)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("search: decode: %w", err)
		}
		return nil
	}
}

// clean strips Brave's <strong> highlighting and HTML entities it leaves.
func clean(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	r := strings.NewReplacer("&amp;", "&", "&quot;", `"`, "&#x27;", "'", "&#39;", "'", "&lt;", "<", "&gt;", ">", "&nbsp;", " ")
	return strings.TrimSpace(r.Replace(s))
}

// ---- rate limit ----

// Limiter spaces calls to at most one per interval (a token bucket of 1):
// callers wait their turn instead of failing.
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

// NewLimiter allows perSecond calls per second (e.g. 1 for Brave's free plan).
func NewLimiter(perSecond float64) *Limiter {
	if perSecond <= 0 {
		return nil
	}
	return &Limiter{interval: time.Duration(float64(time.Second) / perSecond)}
}

// Wait blocks until the caller may make its call.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	l.mu.Lock()
	t := now()
	start := t
	if l.next.After(t) {
		start = l.next
	}
	l.next = start.Add(l.interval)
	l.mu.Unlock()
	if d := start.Sub(t); d > 0 {
		if l.sleep != nil {
			return l.sleep(ctx, d)
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// ---- dev cache ----

// Cached wraps a provider with a file cache (dev and tests only: results
// are stored as plain JSON under dir, e.g. .cache/search).
type Cached struct {
	Provider Provider
	Dir      string
	TTL      time.Duration // 0 = forever
}

// Search serves a cached result for the same query, or asks the provider.
func (c *Cached) Search(ctx context.Context, q Query) ([]Result, error) {
	key, _ := json.Marshal(struct {
		V int
		Q Query
	}{cacheVersion, q})
	sum := sha256.Sum256(key)
	path := filepath.Join(c.Dir, hex.EncodeToString(sum[:16])+".json")
	if info, err := os.Stat(path); err == nil && (c.TTL == 0 || time.Since(info.ModTime()) < c.TTL) {
		if data, err := os.ReadFile(path); err == nil { //nolint:gosec // G304: path is a hash under our cache dir
			var out []Result
			if json.Unmarshal(data, &out) == nil {
				return out, nil
			}
		}
	}
	out, err := c.Provider.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.Dir, 0o750); err == nil {
		if data, err := json.Marshal(out); err == nil {
			_ = os.WriteFile(path, data, 0o600)
		}
	}
	return out, nil
}

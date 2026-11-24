package look

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// Fields of a list row that may hold words from a web page or an email
// (a search result's title, a snippet): they stay untrusted. Everything
// else in a row was pulled out or saved by a brick: extracted.
var rowUntrusted = map[string]bool{"title": true, "snippet": true, "text": true, "page_title": true, "subject": true}

// MyStuff is the Read my stuff brick (look.my_stuff): the rows of one of
// the user's Lists as cards, up to the run's card limit (by default only
// rows this helper hasn't used yet, oldest first; else the newest), or text
// they pasted as one card, or links they pasted as one card each.
type MyStuff struct {
	Lists store.ListStore
}

func (m MyStuff) Run(ctx context.Context, rc *engine.RunContext, _ []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if cfg.String("source") == "paste" {
		text := strings.TrimSpace(cfg.String("text"))
		if text == "" {
			return nil, nil
		}
		c := engine.Card{Kind: "row"}
		c.Set("text", text, engine.TrustUser) // typed (or pasted) by the user
		return []engine.Card{c}, nil
	}
	// Pasted links: each is its own card, so Read the page can open it.
	if cfg.String("source") == "links" {
		links := pastedLinks(cfg.String("links"))
		out := make([]engine.Card, 0, len(links))
		for _, u := range links[:min(len(links), max(1, rc.Limits.MaxCards))] {
			c := engine.Card{Kind: "row", Source: u}
			c.Set("url", u, engine.TrustUser)
			out = append(out, c)
		}
		return out, nil
	}
	if m.Lists == nil {
		return nil, queue.Permanent(errors.New("lists aren't set up"))
	}
	list, err := m.Lists.GetList(ctx, cfg.String("list"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && list.WorkspaceID != rc.Run.WorkspaceID) {
		return nil, queue.Permanent(errors.New("this list doesn't exist anymore; pick another in the brick"))
	}
	if err != nil {
		return nil, err
	}
	// Only new rows (default): the oldest ones this helper hasn't used, so
	// a lead isn't written to twice. A test run looks without using them up.
	var rows []store.ListRow
	if cfg.Bool("only_new") {
		rows, err = m.Lists.TakeListRows(ctx, list.ID, rc.Run.HelperID, rc.Run.ID, max(1, rc.Limits.MaxCards), !rc.TestMode)
	} else {
		rows, err = m.Lists.ListRows(ctx, list.ID, max(1, rc.Limits.MaxCards))
	}
	if err != nil {
		return nil, err
	}
	out := make([]engine.Card, 0, len(rows))
	for _, r := range rows {
		c := engine.Card{Kind: "row"}
		for k, v := range r.Data {
			trust := engine.TrustExtracted
			if rowUntrusted[k] {
				trust = engine.TrustUntrusted
			}
			c.Set(k, v, trust)
		}
		if u, _ := r.Data["url"].(string); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			c.Source = u
		}
		c.Set("list_row_id", r.ID, engine.TrustUser)
		out = append(out, c)
	}
	rc.Log.Info("read list", "list", list.ID, "rows", len(out))
	return out, nil
}

// pastedLinks returns the links in text (one per line, or with spaces
// between), each once and in order. Words that aren't links are left out.
func pastedLinks(text string) []string {
	var out []string
	for _, w := range strings.Fields(text) {
		w = strings.TrimRight(w, ",;")
		if !strings.HasPrefix(w, "https://") && !strings.HasPrefix(w, "http://") {
			continue
		}
		if u, err := url.Parse(w); err != nil || u.Host == "" || slices.Contains(out, w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

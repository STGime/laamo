package look

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/web"
)

// PageReader reads one page (web.Fetcher, or a fake).
type PageReader interface {
	Read(ctx context.Context, url string) (web.Page, error)
}

// ReadPage is the Read a webpage brick (look.read_page).
type ReadPage struct {
	Reader PageReader
	// Parallel pages per run (default 4).
	Parallel int
}

// maxPageText caps page_text with contact pages added (runes).
const maxPageText = 30000

func (b ReadPage) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if b.Reader == nil {
		return nil, queue.Permanent(errors.New("reading web pages isn't set up"))
	}
	depth := cfg.String("depth")
	if cfg.String("source") == "fixed" {
		resolved, err := tmpl.Config(rc, rc.Brick, cfg, nil)
		if err != nil {
			return nil, err
		}
		u := strings.TrimSpace(resolved.String("url"))
		title, text, final, err := b.read(ctx, u, depth)
		if err != nil {
			if web.Transient(err) {
				return nil, fmt.Errorf("read %s: %w", u, err)
			}
			return nil, queue.Permanent(fmt.Errorf("couldn't read %s: %w", u, err))
		}
		c := engine.Card{Kind: "page", Source: final}
		c.Set("page_title", title, engine.TrustUntrusted)
		c.Set("page_text", text, engine.TrustUntrusted)
		return []engine.Card{c}, nil
	}

	field := "website"
	if cfg.String("source") == "url" {
		field = "url"
	}
	out := make([]engine.Card, len(in))
	errs := make([]error, len(in))
	parallel := b.Parallel
	if parallel <= 0 {
		parallel = 4
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i := range in {
		out[i] = in[i].Clone()
		u := strings.TrimSpace(in[i].Text(field))
		if u == "" {
			out[i].Set("page_title", "", engine.TrustUntrusted)
			out[i].Set("page_text", note(rc, "no_site", nil), engine.TrustUntrusted)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			title, text, _, err := b.read(ctx, u, depth)
			if err != nil {
				errs[i] = err
				rc.Log.Info("read page", "url", u, "err", err)
				text = note(rc, "unreadable", err)
			}
			out[i].Set("page_title", title, engine.TrustUntrusted)
			out[i].Set("page_text", text, engine.TrustUntrusted)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Every page failed for a reason that may pass (network down, timeouts):
	// retry the step rather than carry on with nothing.
	failed, transient := 0, 0
	for _, err := range errs {
		if err != nil {
			failed++
			if web.Transient(err) {
				transient++
			}
		}
	}
	if failed > 0 && failed == transient && failed == countWith(in, field) {
		return nil, fmt.Errorf("no page could be read: %w", firstErr(errs))
	}
	return out, nil
}

// read reads a page and, for depth "contact", up to two contact/Impressum
// pages on the same site, and lists the emails and phone numbers found.
func (b ReadPage) read(ctx context.Context, u, depth string) (title, text, final string, err error) {
	p, err := b.Reader.Read(ctx, u)
	if err != nil {
		return "", "", "", err
	}
	var sb strings.Builder
	sb.WriteString(p.Text)
	emails, phones := p.Emails, p.Phones
	if depth == "contact" {
		for _, cu := range web.ContactPages(p, 2) {
			cp, err := b.Reader.Read(ctx, cu)
			if err != nil {
				continue // the main page is enough
			}
			fmt.Fprintf(&sb, "\n\n--- %s ---\n%s", cu, cp.Text)
			emails = appendNew(emails, cp.Emails...)
			phones = appendNew(phones, cp.Phones...)
		}
	}
	if len(emails) > 0 || len(phones) > 0 {
		sb.WriteString("\n\n---\n")
		if len(emails) > 0 {
			fmt.Fprintf(&sb, "Email links: %s\n", strings.Join(emails, ", "))
		}
		if len(phones) > 0 {
			fmt.Fprintf(&sb, "Phone links: %s\n", strings.Join(phones, ", "))
		}
	}
	text = strings.TrimSpace(sb.String())
	if r := []rune(text); len(r) > maxPageText {
		text = string(r[:maxPageText]) + " …"
	}
	return p.Title, text, p.URL, nil
}

func note(rc *engine.RunContext, kind string, err error) string {
	de := rc.Workspace.Locale == "de"
	switch {
	case kind == "no_site" && de:
		return "(Keine Website angegeben.)"
	case kind == "no_site":
		return "(No website given.)"
	case de:
		return "(Die Seite konnte nicht gelesen werden: " + reason(err, true) + ")"
	default:
		return "(The page couldn't be read: " + reason(err, false) + ")"
	}
}

func reason(err error, de bool) string {
	var se *web.StatusError
	switch {
	case errors.Is(err, web.ErrRobots):
		return pick(de, "Die Seite bittet Bots, sie nicht zu lesen.", "the site asks bots not to read it.")
	case errors.Is(err, web.ErrNotPage):
		return pick(de, "Das ist keine Webseite.", "it isn't a web page.")
	case errors.Is(err, web.ErrBlocked), errors.Is(err, web.ErrBadURL):
		return pick(de, "Das ist keine öffentliche Webadresse.", "it isn't a public web address.")
	case errors.As(err, &se) && (se.Status == 401 || se.Status == 403):
		return pick(de, "Die Website lässt keine Bots herein (Bezahlschranke oder Bot-Schutz).", "the site doesn't let bots in (paywall or bot protection).")
	case errors.As(err, &se) && se.Status == 404:
		return pick(de, "Die Seite gibt es nicht (404).", "the page doesn't exist (404).")
	default:
		return pick(de, "Die Website war nicht erreichbar.", "the website couldn't be reached.")
	}
}

func pick(de bool, d, e string) string {
	if de {
		return d
	}
	return e
}

func appendNew(list []string, more ...string) []string {
	for _, m := range more {
		dup := false
		for _, l := range list {
			if strings.EqualFold(l, m) {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, m)
		}
	}
	return list
}

func countWith(in []engine.Card, field string) int {
	n := 0
	for _, c := range in {
		if strings.TrimSpace(c.Text(field)) != "" {
			n++
		}
	}
	return n
}

func firstErr(errs []error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

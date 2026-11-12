package engine

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// Learning from swipes (#76). With "Learn from my choices" on, what the
// user keeps or skips on a Let me pick card is remembered for the helper.
// A Keep the good ones brick before it then shows the AI the newest ones as
// labelled examples, so results get closer to the user's taste every run.
//
// The kept/skipped label is the user's own choice; the card text is web
// content and stays data (Keep sends it inside the <cards> guard). Nothing
// is learned from test runs or from cards that expired unanswered, and
// turning the setting off stops both remembering and using examples.

// LearnedPerKind is how many kept and how many skipped examples Keep the
// good ones sees.
const LearnedPerKind = 8

// learnsFrom reports whether a stack uses learned examples: a Keep the good
// ones brick with a Let me pick after it that remembers choices.
func learnsFrom(cat *catalog.Catalog, stack catalog.Stack) bool {
	keep := slices.IndexFunc(stack.Bricks, func(b catalog.StackBrick) bool { return b.Type == "think.keep" })
	if keep < 0 {
		return false
	}
	return slices.ContainsFunc(stack.Bricks[keep+1:], func(sb catalog.StackBrick) bool {
		return sb.Type == "ask.pick" && remembers(cat, sb)
	})
}

func remembers(cat *catalog.Catalog, sb catalog.StackBrick) bool {
	b, ok := cat.Brick(sb.Type)
	return ok && Config(b.Config(sb.Config)).Bool("remember")
}

// learned loads a helper's newest examples, kept ones first.
func (e *Engine) learned(ctx context.Context, helperID string) ([]store.PickExample, error) {
	kept, err := e.Store.PickExamples(ctx, helperID, true, LearnedPerKind)
	if err != nil {
		return nil, err
	}
	skipped, err := e.Store.PickExamples(ctx, helperID, false, LearnedPerKind)
	if err != nil {
		return nil, err
	}
	return append(kept, skipped...), nil
}

// remember records a Let me pick decision as examples.
func (e *Engine) remember(ctx context.Context, r store.Run, stepID string, shown []Card, a applied) error {
	ex := make([]store.PickExample, 0, len(shown))
	for _, c := range shown {
		title := firstText(c, "title", "name", "page_title", "subject")
		text := firstText(c, "snippet", "summary", "text", "address")
		if title == "" && text == "" {
			continue
		}
		ex = append(ex, store.PickExample{WorkspaceID: r.WorkspaceID, UserID: r.UserID, HelperID: r.HelperID,
			StepID: stepID, CardID: c.ID, Kept: slices.Contains(a.Keep, c.ID),
			Title: cutRunes(title, 300), Site: site(c), Text: cutRunes(text, 600)})
	}
	return e.Store.AddPickExamples(ctx, ex)
}

func firstText(c Card, fields ...string) string {
	for _, f := range fields {
		if s := strings.TrimSpace(c.Text(f)); s != "" {
			return s
		}
	}
	return ""
}

// site is the card's web address without "www.", e.g. "baeckerei-klein.de".
func site(c Card) string {
	u, err := url.Parse(firstText(c, "url", "website"))
	if err != nil {
		return ""
	}
	return cutRunes(strings.TrimPrefix(u.Hostname(), "www."), 200)
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

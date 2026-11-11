// Package shelf turns Shelf templates into helpers: a plain-words preview
// for a workspace, the setup questions it still has to ask, and the stack
// a helper gets from the answers.
package shelf

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// FeedOnly is the answer to a "where should I tell you?" question that
// keeps results in the Feed: the Post to chat bricks it would fill are left
// out of the helper.
const FeedOnly = "feed"

// ErrAnswer: a setup answer is missing or doesn't fit.
var ErrAnswer = errors.New("setup answer")

// Brick is one brick of the preview.
type Brick struct {
	ID       string
	Type     string
	Group    string
	Sentence string
}

// Question is a setup question the user still has to answer.
type Question struct {
	Key         string // the profile fact for About me questions
	Kind        string // profile, text, select, place, list, time
	Profile     bool
	Text        string
	Placeholder string
	Fills       []string // brick ids
}

// View is a template as the workspace sees it.
type View struct {
	Template  catalog.Template
	Name      string
	Promise   string
	Bricks    []Brick
	Questions []Question
	Brain     bool
	Places    []string // place types any of which works ("" = none needed)
	Paid      bool
	// Match ranks the template for the workspace's needs (0 = no match).
	Match int
}

var profilePill = regexp.MustCompile(`\{my ([a-z_]+)\}`)

// Preview describes a template for a workspace: bricks in plain words with
// known About me facts filled in, and only the questions still open.
func Preview(cat *catalog.Catalog, t catalog.Template, ws store.Workspace) View {
	lang := ws.Locale
	v := View{Template: t, Name: t.Name.In(lang), Promise: t.Promise.In(lang)}
	stack := t.StackFor(lang)
	v.Bricks = Describe(cat, stack, ws)
	for _, q := range t.SetupQuestions {
		if q.Profile != "" {
			if known(ws.Profile, q.Profile) {
				continue
			}
			v.Questions = append(v.Questions, Question{Key: q.Profile, Kind: "text", Profile: true, Text: profileQuestion(cat, q.Profile, lang)})
			continue
		}
		pq := Question{Key: q.Key, Kind: q.Type}
		for _, f := range q.Fills {
			pq.Fills = append(pq.Fills, f.Brick)
		}
		if q.Question != nil {
			pq.Text = q.Question.In(lang)
		}
		if q.Placeholder != nil {
			pq.Placeholder = q.Placeholder.In(lang)
		}
		v.Questions = append(v.Questions, pq)
	}
	need := cat.Requirements(stack)
	v.Brain, v.Paid = need.Brain, need.Paid
	if len(need.Places) > 0 {
		v.Places = need.Places[0]
	}
	if i := slices.Index(ws.Needs, t.Category); i >= 0 {
		v.Match = len(ws.Needs) - i // the first need ranks highest
	}
	return v
}

// Describe puts a stack's bricks in plain words for a workspace, with known
// About me facts filled in.
func Describe(cat *catalog.Catalog, stack catalog.Stack, ws store.Workspace) []Brick {
	out := make([]Brick, 0, len(stack.Bricks))
	profile := stack.WithProfile(ws.Profile)
	for _, sb := range stack.Bricks {
		b, _ := cat.Brick(sb.Type)
		group := ""
		if b != nil {
			group = b.Group
		}
		out = append(out, Brick{ID: sb.ID, Type: sb.Type, Group: group, Sentence: fillProfile(cat.Sentence(sb, ws.Locale), profile)})
	}
	return out
}

// Sort orders templates for a workspace: matching needs first (in the
// order picked), then by the template's sort order.
func Sort(views []View) {
	slices.SortStableFunc(views, func(a, b View) int {
		if a.Match != b.Match {
			return b.Match - a.Match
		}
		return a.Template.SortOrder - b.Template.SortOrder
	})
}

// Install builds the stack a helper gets from a template and the answers,
// and the About me facts to remember (asked once, used by every helper).
// Facts named in just stay with this helper (stack.Profile) instead.
// It checks the stack against the catalog.
func Install(cat *catalog.Catalog, t catalog.Template, ws store.Workspace, answers map[string]string, just []string,
	place func(id string) (store.Place, error)) (catalog.Stack, map[string]any, error) {
	stack := t.StackFor(ws.Locale)
	profile := map[string]any{}
	bad := func(format string, a ...any) error { return fmt.Errorf("%w: %s", ErrAnswer, fmt.Sprintf(format, a...)) }
	var drop []string
	for _, q := range t.SetupQuestions {
		if q.Profile != "" {
			if known(ws.Profile, q.Profile) {
				continue
			}
			v := strings.TrimSpace(answers[q.Profile])
			if v == "" {
				return stack, nil, bad("%s is missing", q.Profile)
			}
			if utf8.RuneCountInString(v) > 200 {
				return stack, nil, bad("%s is too long", q.Profile)
			}
			if slices.Contains(just, q.Profile) {
				if stack.Profile == nil {
					stack.Profile = map[string]string{}
				}
				stack.Profile[q.Profile] = v
			} else {
				profile[q.Profile] = v
			}
			continue
		}
		v := strings.TrimSpace(answers[q.Key])
		if v == "" {
			return stack, nil, bad("%s is missing", q.Key)
		}
		if q.Type == "place" {
			if v == FeedOnly {
				for _, f := range q.Fills {
					drop = append(drop, f.Brick)
				}
				continue
			}
			p, err := place(v)
			if err != nil || p.WorkspaceID != ws.ID {
				return stack, nil, bad("unknown place")
			}
		}
		for _, f := range q.Fills {
			i := slices.IndexFunc(stack.Bricks, func(b catalog.StackBrick) bool { return b.ID == f.Brick })
			if i < 0 {
				continue
			}
			if stack.Bricks[i].Config == nil {
				stack.Bricks[i].Config = map[string]any{}
			}
			stack.Bricks[i].Config[f.Setting] = v
		}
	}
	stack.Bricks = slices.DeleteFunc(stack.Bricks, func(b catalog.StackBrick) bool { return slices.Contains(drop, b.ID) })
	if errs := catalog.Errors(cat.Validate(stack, catalog.Options{})); len(errs) > 0 {
		return stack, nil, bad("%s", errs[0].Message)
	}
	return stack, profile, nil
}

// Find returns the template with this slug.
func Find(slug string) (catalog.Template, bool) {
	ts, err := catalog.Templates()
	if err != nil {
		return catalog.Template{}, false
	}
	i := slices.IndexFunc(ts, func(t catalog.Template) bool { return t.Slug == slug })
	if i < 0 {
		return catalog.Template{}, false
	}
	return ts[i], true
}

func known(profile map[string]any, key string) bool {
	s, _ := profile[key].(string)
	return strings.TrimSpace(s) != ""
}

func fillProfile(text string, profile map[string]any) string {
	return profilePill.ReplaceAllStringFunc(text, func(m string) string {
		key := profilePill.FindStringSubmatch(m)[1]
		if s, ok := profile[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
		return m
	})
}

func profileQuestion(cat *catalog.Catalog, key, lang string) string {
	for _, p := range cat.Profile {
		if p.Key == key {
			return p.Question.In(lang)
		}
	}
	return key
}

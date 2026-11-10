package catalog_test

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	files "github.com/STGime/laamo/packages/catalog"

	"github.com/STGime/laamo/backend/internal/catalog"
)

var cat = catalog.Default()

// ---- the catalog file itself ----

func TestCatalogConsistency(t *testing.T) {
	colors := map[string]bool{}
	for _, g := range cat.Groups {
		if colors[g.Color] {
			t.Errorf("color %s used twice", g.Color)
		}
		colors[g.Color] = true
	}
	known := map[string]bool{} // fields some brick can add, or listed in fields
	for _, f := range cat.Fields {
		known[f.Key] = true
	}
	for _, b := range cat.Bricks {
		known = addAll(known, b.Adds)
		for _, r := range b.AddsIf {
			known = addAll(known, r.Fields)
		}
	}

	refRe := regexp.MustCompile(`\[([a-z_]+)\]`)
	for i := range cat.Bricks {
		b := &cat.Bricks[i]
		t.Run(b.ID, func(t *testing.T) {
			if _, ok := cat.Group(b.Group); !ok || !strings.HasPrefix(b.ID, b.Group+".") {
				t.Errorf("group %q doesn't match id", b.Group)
			}
			if (b.Group == "when") != (b.Shape == catalog.ShapeStart) || (b.Group == "when") != (b.Trigger != "") {
				t.Errorf("When bricks (and only they) have shape start and a trigger")
			}
			if last := b.Sentence[len(b.Sentence)-1]; len(last.If) != 0 {
				t.Errorf("the last sentence needs no condition (fallback)")
			}
			for _, s := range b.Sentence {
				for _, lang := range []string{s.Text.En, s.Text.De} {
					for _, m := range refRe.FindAllStringSubmatch(lang, -1) {
						if _, ok := b.Setting(m[1]); !ok {
							t.Errorf("sentence uses [%s], no such setting", m[1])
						}
					}
				}
				checkCondition(t, b, "sentence", s.If)
			}
			checkCondition(t, b, "check.required_if", conditionOf(b.Check))
			for _, r := range b.ShapeIf {
				checkCondition(t, b, "shape_if", r.If)
			}
			for _, r := range b.NeedsIf {
				checkCondition(t, b, "needs_if", r.If)
				for _, f := range r.Fields {
					if !known[f] {
						t.Errorf("needs_if field %q is never added by any brick", f)
					}
				}
			}
			for _, f := range b.Needs {
				if !known[f] {
					t.Errorf("needs field %q is never added by any brick", f)
				}
			}
			for _, r := range b.AddsIf {
				checkCondition(t, b, "adds_if", r.If)
			}
			for _, r := range b.ProvidesIf {
				checkCondition(t, b, "provides_if", r.If)
			}
			for _, r := range b.RequiresIf {
				checkCondition(t, b, "requires_if", r.If)
			}
			if b.AddsFrom != "" {
				if s, ok := b.Setting(b.AddsFrom); !ok || s.Type != catalog.TypeFields {
					t.Errorf("adds_from %q must be a fields setting", b.AddsFrom)
				}
			}

			seen := map[string]bool{}
			for j := range b.Settings {
				s := &b.Settings[j]
				if seen[s.Key] {
					t.Errorf("setting %q twice", s.Key)
				}
				seen[s.Key] = true
				checkCondition(t, b, "show_if "+s.Key, s.ShowIf)
				if s.Placeholder != nil && s.Pills {
					for _, p := range catalog.Pills(s.Placeholder.En + s.Placeholder.De) {
						if p.Kind == "" || (p.Kind == "profile" && !cat.HasProfile(p.Key)) {
							t.Errorf("placeholder pill {%s} is invalid", p.Raw)
						}
					}
				}
			}

			// Defaults must be valid: a fresh brick with only its required
			// text filled in has no setting errors.
			sb := catalog.StackBrick{ID: "b1", Type: b.ID, Config: map[string]any{}}
			st := catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{sb}}
			for _, iss := range cat.Validate(st, catalog.Options{}) {
				if iss.Code == catalog.CodeBadValue || iss.Code == catalog.CodeUnknownSetting {
					t.Errorf("default config: %s", iss.Message)
				}
			}

			// Design rule: at most 3 visible settings (the rest behind "More").
			for _, cfg := range configVariants(b) {
				visible := 0
				for j := range b.Settings {
					if s := &b.Settings[j]; !s.Advanced && s.Visible(b.Config(cfg)) {
						visible++
					}
				}
				if visible > 3 {
					t.Errorf("%d visible settings with %v; max 3", visible, cfg)
				}
			}
		})
	}

	// Fixes insert a brick that really adds the missing field.
	for field, fix := range cat.Fixes {
		b, ok := cat.Brick(fix.Brick)
		if !ok {
			t.Errorf("fix %s: unknown brick %s", field, fix.Brick)
			continue
		}
		for k := range fix.Config {
			if _, ok := b.Setting(k); !ok {
				t.Errorf("fix %s: %s has no setting %q", field, b.ID, k)
			}
		}
		if field == "check" {
			continue
		}
		adds := addedBy(b, fix.Config)
		if !slices.Contains(adds, field) {
			t.Errorf("fix %s: %s with %v doesn't add %s (adds %v)", field, b.ID, fix.Config, field, adds)
		}
	}

	pick := 0
	for _, f := range cat.Fields {
		if f.Pick {
			pick++
		}
	}
	if pick < 5 {
		t.Errorf("only %d pickable fields for Pull out details", pick)
	}
	mvp := 0
	for _, b := range cat.Bricks {
		if b.Status == "mvp" {
			mvp++
		}
	}
	if len(cat.Bricks) != 21 || mvp != 19 {
		t.Errorf("want 21 bricks (19 mvp + Something changed + Posta), got %d (%d mvp)", len(cat.Bricks), mvp)
	}
}

func addAll(m map[string]bool, keys []string) map[string]bool {
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// addedBy runs a one-brick enrich check: which fields does b add with cfg?
func addedBy(b *catalog.Brick, cfg map[string]any) []string {
	full := b.Config(cfg)
	out := slices.Clone(b.Adds)
	for _, r := range b.AddsIf {
		if r.If.Matches(full) {
			out = append(out, r.Fields...)
		}
	}
	if b.AddsFrom != "" {
		if list, ok := full[b.AddsFrom].([]any); ok {
			for _, v := range list {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func conditionOf(c *catalog.CheckRule) catalog.Condition {
	if c == nil {
		return nil
	}
	return c.RequiredIf
}

// checkCondition: keys are settings of the brick; select values exist.
func checkCondition(t *testing.T, b *catalog.Brick, where string, c catalog.Condition) {
	t.Helper()
	for key, raw := range c {
		s, ok := b.Setting(key)
		if !ok {
			t.Errorf("%s: no setting %q", where, key)
			continue
		}
		var values []any
		if json.Unmarshal(raw, &values) != nil || s.Type != catalog.TypeSelect {
			continue
		}
		for _, v := range values {
			if !slices.ContainsFunc(s.Options, func(o catalog.Option) bool { return o.Value == v }) {
				t.Errorf("%s: %s has no option %v", where, key, v)
			}
		}
	}
}

// configVariants: defaults, plus each select option and toggle value one at a time.
func configVariants(b *catalog.Brick) []map[string]any {
	out := []map[string]any{{}}
	for _, s := range b.Settings {
		switch s.Type {
		case catalog.TypeSelect:
			for _, o := range s.Options {
				out = append(out, map[string]any{s.Key: o.Value})
			}
		case catalog.TypeToggle:
			out = append(out, map[string]any{s.Key: true}, map[string]any{s.Key: false})
		}
	}
	return out
}

// ---- templates ----

func TestTemplates(t *testing.T) {
	templates, err := catalog.Templates()
	if err != nil {
		t.Fatal(err)
	}
	names, _ := fs.Glob(files.Files, "templates/*.json")
	if len(templates) == 0 || len(templates) != len(names) {
		t.Fatalf("templates: %d loaded, %d files", len(templates), len(names))
	}
	for _, tpl := range templates {
		t.Run(tpl.Slug, func(t *testing.T) {
			if !slices.Contains(names, "templates/"+tpl.Slug+".json") {
				t.Errorf("slug must match the file name")
			}
			pending := map[string]bool{}
			for _, q := range tpl.SetupQuestions {
				switch {
				case q.Profile != "":
					if !cat.HasProfile(q.Profile) {
						t.Errorf("setup question: no About me fact %q", q.Profile)
					}
				default:
					for _, f := range q.Fills {
						i := slices.IndexFunc(tpl.Stack.Bricks, func(sb catalog.StackBrick) bool { return sb.ID == f.Brick })
						if i < 0 {
							t.Errorf("setup question %s fills unknown brick %s", q.Key, f.Brick)
							continue
						}
						b, _ := cat.Brick(tpl.Stack.Bricks[i].Type)
						if _, ok := b.Setting(f.Setting); !ok {
							t.Errorf("setup question %s: %s has no setting %s", q.Key, b.ID, f.Setting)
						}
						pending[f.Brick+"."+f.Setting] = true
					}
				}
			}
			if errs := catalog.Errors(cat.Validate(tpl.Stack, catalog.Options{Pending: pending})); len(errs) > 0 {
				for _, e := range errs {
					t.Errorf("%s: %s", e.Brick, e.Message)
				}
			}
			need := cat.Requirements(tpl.Stack)
			if need.Brain != tpl.Required.Brain || need.Paid != (tpl.Required.Plan == "paid") {
				t.Errorf("required %+v doesn't match the stack's needs %+v", tpl.Required, need)
			}
			switch len(need.Places) {
			case 0:
				if len(tpl.Required.Places) > 0 {
					t.Errorf("required places %v, but no brick needs a place", tpl.Required.Places)
				}
			case 1:
				if !slices.Equal(need.Places[0], slices.Sorted(slices.Values(tpl.Required.Places))) {
					t.Errorf("required places %v, stack needs one of %v", tpl.Required.Places, need.Places[0])
				}
			}
		})
	}
}

// ---- the validator ----

type b = catalog.StackBrick
type cfg = map[string]any

func stack(bricks ...b) catalog.Stack {
	for i := range bricks {
		if bricks[i].ID == "" {
			bricks[i].ID = "b" + string(rune('1'+i))
		}
	}
	return catalog.Stack{Schema: 1, Bricks: bricks}
}

func codes(issues []catalog.Issue) []string {
	out := []string{}
	for _, i := range issues {
		out = append(out, i.Brick+":"+i.Code)
	}
	slices.Sort(out)
	return out
}

func TestValidate(t *testing.T) {
	every := b{Type: "when.every", Config: cfg{"freq": "weekly", "days": []string{"mon"}, "time": "08:00"}}
	tap := b{Type: "when.tap"}
	search := b{Type: "look.search", Config: cfg{"query": "bakery news"}}

	cases := []struct {
		name string
		st   catalog.Stack
		want []string
	}{
		{"morning briefing shape", stack(every, search,
			b{Type: "think.keep", Config: cfg{"criteria": "matter to {my business}", "max": 5}},
			b{Type: "think.summarize"},
			b{Type: "do.chat", Config: cfg{"place": "p1"}}), nil},
		{"customer finder without extract or check", stack(every,
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries"}},
			b{Type: "look.read_page", Config: cfg{"depth": "contact"}},
			b{Type: "think.write", Config: cfg{"what": "a friendly intro"}},
			b{Type: "do.email", Config: cfg{"to": "cards"}}),
			[]string{"b5:check_required", "b5:missing_field"}},
		{"customer finder complete", stack(every,
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries"}},
			b{Type: "look.read_page", Config: cfg{"depth": "contact"}},
			b{Type: "think.extract", Config: cfg{"fields": []string{"email"}}},
			b{Type: "think.keep", Config: cfg{"criteria": "no online shop"}},
			b{Type: "ask.pick"},
			b{Type: "think.write", Config: cfg{"what": "a friendly intro offering {my offer} to {name}"}},
			b{Type: "ask.check"},
			b{Type: "do.email", Config: cfg{"to": "cards", "subject": "Hello {name}"}},
			b{Type: "do.list", Config: cfg{"list": "leads", "dedupe": "email"}}), nil},
		{"must start with When", stack(search), []string{"b1:start_first"}},
		{"When only on top", stack(tap, every), []string{"b2:start_only_first"}},
		{"unknown and later bricks", stack(tap, b{Type: "look.magic"}, b{Type: "do.social", Config: cfg{"accounts": "x"}}),
			[]string{"b2:unknown_brick", "b3:check_suggested", "b3:missing_field", "b3:not_available"}},
		{"settings", stack(
			b{Type: "when.every", Config: cfg{"freq": "weekly", "days": []string{}, "time": "8am", "color": "red"}},
			b{Type: "look.search", Config: cfg{"count": 7}},
			b{Type: "think.keep", Config: cfg{"criteria": "x", "max": 99}}),
			[]string{"b1:bad_value", "b1:missing_setting", "b1:unknown_setting", "b2:bad_value", "b2:missing_setting", "b3:bad_value"}},
		{"hidden settings are not required", stack(tap,
			b{Type: "look.my_stuff", Config: cfg{"source": "paste", "text": "a, b"}},
			b{Type: "look.read_page", Config: cfg{"source": "fixed", "url": "https://example.com"}}), nil},
		{"later option", stack(tap, b{Type: "look.my_stuff", Config: cfg{"source": "file"}}), []string{"b2:bad_value"}},
		{"pills", stack(tap,
			b{Type: "look.search", Config: cfg{"query": "{my planet} {answer} {} {email}"}}),
			[]string{"b2:bad_pill", "b2:missing_context", "b2:missing_field", "b2:unknown_profile"}},
		{"answer from Tap Go question", stack(
			b{Type: "when.tap", Config: cfg{"question": "Which city?"}},
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries", "where": "{answer}"}}), nil},
		{"answer from Ask me something", stack(tap,
			b{Type: "ask.question", Config: cfg{"question": "Which city?"}},
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries", "where": "{answer}"}}), nil},
		{"trigger only with email", stack(tap,
			b{Type: "think.write", Config: cfg{"what": "a reply to {trigger.subject}"}}), []string{"b2:missing_context"}},
		{"meeting catcher", stack(
			b{Type: "when.email"},
			b{Type: "think.extract", Config: cfg{"fields": []string{"date", "time", "place", "name"}}},
			b{Type: "ask.check"},
			b{Type: "do.calendar", Config: cfg{"invite_others": false}},
			b{Type: "think.write", Config: cfg{"what": "a short confirmation for {trigger.subject}"}},
			b{Type: "do.email", Config: cfg{"to": "sender"}}), nil},
		{"reply needs the email trigger", stack(tap,
			b{Type: "think.write", Config: cfg{"what": "hi"}},
			b{Type: "ask.check"},
			b{Type: "do.email", Config: cfg{"to": "sender"}}), []string{"b4:missing_context"}},
		{"combine drops fields", stack(every,
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries"}},
			b{Type: "think.summarize", Config: cfg{"mode": "all"}},
			b{Type: "do.list", Config: cfg{"list": "l", "dedupe": "website"}}), []string{"b4:missing_field"}},
		{"enrich keeps fields", stack(every,
			b{Type: "look.find_businesses", Config: cfg{"what": "bakeries"}},
			b{Type: "think.summarize", Config: cfg{"mode": "each"}},
			b{Type: "do.list", Config: cfg{"list": "l", "dedupe": "website"}}), nil},
		{"list columns are open", stack(tap,
			b{Type: "look.my_stuff", Config: cfg{"list": "leads"}},
			b{Type: "think.write", Config: cfg{"what": "hi {name}"}},
			b{Type: "ask.check"},
			b{Type: "do.email", Config: cfg{"to": "cards"}}), nil},
		{"email me a summary", stack(every, search, b{Type: "think.summarize"}, b{Type: "do.email"}), nil},
		{"check that goes ahead", stack(tap,
			b{Type: "look.my_stuff", Config: cfg{"list": "leads"}},
			b{Type: "ask.check", Config: cfg{"on_timeout": "go"}},
			b{Type: "think.write", Config: cfg{"what": "hi"}},
			b{Type: "do.email", Config: cfg{"to": "cards"}}), []string{"b3:check_goes_ahead"}},
		{"invite others needs email and check", stack(tap,
			b{Type: "look.my_stuff", Config: cfg{"source": "paste", "text": "x"}},
			b{Type: "think.extract", Config: cfg{"fields": []string{"date", "time"}}},
			b{Type: "do.calendar", Config: cfg{"invite_others": true}}),
			[]string{"b4:check_required", "b4:missing_field"}},
		{"Go ints and float settings", stack(tap,
			b{Type: "look.search", Config: cfg{"query": "x", "count": 25}},
			b{Type: "ask.stop", Config: cfg{"max_cards": 20, "max_cost_eur": 0.25}}), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codes(cat.Validate(tc.st, catalog.Options{}))
			want := tc.want
			if want == nil {
				want = []string{}
			}
			slices.Sort(want)
			if !slices.Equal(got, want) {
				for _, i := range cat.Validate(tc.st, catalog.Options{}) {
					t.Logf("  %s %s: %s", i.Brick, i.Code, i.Message)
				}
				t.Fatalf("issues\n got  %v\n want %v", got, want)
			}
		})
	}
}

func TestStackLevelIssues(t *testing.T) {
	bad := 0
	st := catalog.Stack{Schema: 2, Limits: &catalog.Limits{MaxCards: &bad}}
	got := codes(cat.Validate(st, catalog.Options{}))
	if !slices.Equal(got, []string{":bad_schema", ":empty_stack"}) {
		t.Fatalf("got %v", got)
	}
	st = stack(b{ID: "b1", Type: "when.tap"}, b{ID: "b1", Type: "ask.pick"})
	st.Limits = &catalog.Limits{MaxCards: &bad}
	if got := codes(cat.Validate(st, catalog.Options{})); !slices.Equal(got, []string{":bad_limit", "b1:duplicate_id"}) {
		t.Fatalf("got %v", got)
	}
}

// Applying the suggested fixes makes the stack valid: the builder's one-tap
// fix and Talk to build's repair loop rely on this.
func TestFixesResolveIssues(t *testing.T) {
	st := stack(
		b{Type: "when.every", Config: cfg{"freq": "weekdays", "time": "09:00"}},
		b{Type: "look.find_businesses", Config: cfg{"what": "dentists"}},
		b{Type: "think.write", Config: cfg{"what": "an intro"}},
		b{Type: "do.email", Config: cfg{"to": "cards"}},
	)
	for round := range 5 {
		issues := catalog.Errors(cat.Validate(st, catalog.Options{}))
		if len(issues) == 0 {
			return
		}
		fix := issues[0].Fix
		if fix == nil {
			t.Fatalf("no fix for %s", issues[0].Message)
		}
		i := slices.IndexFunc(st.Bricks, func(x b) bool { return x.ID == fix.Before })
		st.Bricks = slices.Insert(st.Bricks, i, b{ID: "f" + string(rune('0'+round)), Type: fix.Insert, Config: fix.Config})
	}
	t.Fatalf("fixes did not converge: %v", codes(cat.Validate(st, catalog.Options{})))
}

func TestPendingSettings(t *testing.T) {
	st := stack(b{Type: "when.tap"}, b{Type: "do.chat"})
	if got := codes(cat.Validate(st, catalog.Options{})); !slices.Equal(got, []string{"b2:missing_setting"}) {
		t.Fatalf("got %v", got)
	}
	if got := cat.Validate(st, catalog.Options{Pending: map[string]bool{"b2.place": true}}); len(got) != 0 {
		t.Fatalf("pending setting still reported: %v", codes(got))
	}
}

func TestRequirements(t *testing.T) {
	st := stack(b{Type: "when.email"},
		b{Type: "think.write", Config: cfg{"what": "a reply"}},
		b{Type: "ask.check"},
		b{Type: "do.email", Config: cfg{"to": "sender"}},
		b{Type: "do.chat", Config: cfg{"place": "p"}})
	n := cat.Requirements(st)
	if !n.Brain || !n.Paid || len(n.Places) != 1 || !slices.Equal(n.Places[0], []string{"discord", "slack", "teams", "telegram"}) {
		t.Fatalf("needs: %+v", n)
	}
	if n := cat.Requirements(stack(b{Type: "when.tap"}, b{Type: "do.email"})); n.Brain || n.Paid || len(n.Places) != 0 {
		t.Fatalf("email me needs nothing: %+v", n)
	}
}

func TestSentence(t *testing.T) {
	cases := []struct {
		sb   b
		lang string
		want string
	}{
		{b{Type: "when.every", Config: cfg{"freq": "weekly", "days": []string{"mon", "fri"}, "time": "08:00"}}, "en", "Every Monday and Friday at 08:00"},
		{b{Type: "when.every", Config: cfg{"freq": "weekly", "days": []string{"mon", "wed", "fri"}, "time": "08:00"}}, "de", "Jeden Montag, Mittwoch und Freitag um 08:00"},
		{b{Type: "when.every", Config: cfg{"time": "07:30"}}, "en", "Every weekday at 07:30"},
		{b{Type: "look.find_businesses", Config: cfg{"what": "bakeries"}}, "en", "Find bakeries in {my city}"},
		{b{Type: "think.extract", Config: cfg{"fields": []string{"email", "phone"}}}, "de", "E-Mail und Telefon herausziehen"},
		{b{Type: "think.keep", Config: cfg{"criteria": "have no shop", "max": 5}}, "en", "Keep ones that have no shop, max 5"},
		{b{Type: "do.email", Config: cfg{"to": "fixed", "address": "a@b.de"}}, "en", "Prepare an email to a@b.de"},
		{b{Type: "look.read_page", Config: cfg{"depth": "contact"}}, "en", "Read their website and contact page"},
		{b{Type: "nope.brick"}, "en", "nope.brick"},
	}
	for _, tc := range cases {
		if got := cat.Sentence(tc.sb, tc.lang); got != tc.want {
			t.Errorf("%s (%s): got %q, want %q", tc.sb.Type, tc.lang, got, tc.want)
		}
	}
}

func TestPills(t *testing.T) {
	got := catalog.Pills("Hi {name}, about {my business} and {trigger.subject} {answer} {Bad Pill}")
	want := []catalog.Pill{
		{Raw: "name", Kind: "field", Key: "name"},
		{Raw: "my business", Kind: "profile", Key: "business"},
		{Raw: "trigger.subject", Kind: "context", Key: "trigger.subject"},
		{Raw: "answer", Kind: "context", Key: "answer"},
		{Raw: "Bad Pill", Kind: "", Key: "Bad Pill"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
}

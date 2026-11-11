package catalog

import (
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Stack is a helper's bricks, as stored on helper_versions.stack.
type Stack struct {
	Schema int          `json:"schema"`
	Bricks []StackBrick `json:"bricks"`
	Limits *Limits      `json:"limits,omitempty"`
}

type StackBrick struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type Limits struct {
	MaxCards          *int     `json:"max_cards,omitempty"`
	MaxExternalEmails *int     `json:"max_external_emails,omitempty"`
	MaxCostEUR        *float64 `json:"max_cost_eur,omitempty"`
}

// Issue levels.
const (
	LevelError = "error" // the stack can't be saved or run
	LevelHint  = "hint"  // a gentle suggestion with a one-tap fix
)

// Issue codes. The UI shows localized text by code; Message is English for
// logs and the API.
const (
	CodeBadSchema      = "bad_schema"
	CodeEmptyStack     = "empty_stack"
	CodeDuplicateID    = "duplicate_id"
	CodeUnknownBrick   = "unknown_brick"
	CodeNotAvailable   = "not_available"
	CodeStartFirst     = "start_first"
	CodeStartOnlyFirst = "start_only_first"
	CodeUnknownSetting = "unknown_setting"
	CodeMissingSetting = "missing_setting"
	CodeBadValue       = "bad_value"
	CodeBadPill        = "bad_pill"
	CodeUnknownProfile = "unknown_profile"
	CodeMissingField   = "missing_field"
	CodeMissingContext = "missing_context"
	CodeCheckRequired  = "check_required"
	CodeCheckGoesAhead = "check_goes_ahead"
	CodeCheckSuggested = "check_suggested"
	CodeBadLimit       = "bad_limit"
)

type Issue struct {
	Brick   string    `json:"brick,omitempty"` // stack brick id ("" = whole stack)
	Index   int       `json:"index"`           // position in the stack, -1 for the whole stack
	Level   string    `json:"level"`
	Code    string    `json:"code"`
	Setting string    `json:"setting,omitempty"`
	Field   string    `json:"field,omitempty"` // missing card field or context key
	Message string    `json:"message"`
	Fix     *IssueFix `json:"fix,omitempty"`
}

// IssueFix: insert brick Insert (with Config) right before brick Before.
type IssueFix struct {
	Insert string         `json:"insert"`
	Config map[string]any `json:"config,omitempty"`
	Before string         `json:"before"`
}

// Options tune validation.
type Options struct {
	// Pending lists settings ("b5.place") that are filled later, e.g. by a
	// template's setup questions; they don't count as missing.
	Pending map[string]bool
}

// Errors returns only the error-level issues.
func Errors(issues []Issue) []Issue {
	return slices.DeleteFunc(slices.Clone(issues), func(i Issue) bool { return i.Level != LevelError })
}

var pillRe = regexp.MustCompile(`\{([^{}]*)\}`)
var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

var weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// Pill is a {…} placeholder in a setting.
type Pill struct {
	Raw  string // e.g. "my city"
	Kind string // field, profile, context
	Key  string // e.g. city, email, answer, trigger.subject
}

// Pills returns the {…} placeholders in s. Bad ones have Kind "".
func Pills(s string) []Pill {
	var out []Pill
	for _, m := range pillRe.FindAllStringSubmatch(s, -1) {
		raw := strings.TrimSpace(m[1])
		p := Pill{Raw: raw}
		switch {
		case strings.HasPrefix(raw, "my "):
			p.Kind, p.Key = "profile", strings.ReplaceAll(strings.TrimSpace(raw[3:]), " ", "_")
		case raw == "answer":
			p.Kind, p.Key = "context", raw
		case strings.HasPrefix(raw, "trigger."):
			p.Kind, p.Key = "context", raw
		default:
			p.Kind, p.Key = "field", raw
		}
		if !keyRe.MatchString(strings.TrimPrefix(p.Key, "trigger.")) {
			p.Kind = ""
		}
		out = append(out, p)
	}
	return out
}

// state is what exists at a point in the stack.
type state struct {
	fields  map[string]bool
	open    bool // unknown fields may exist (e.g. a List's columns)
	context map[string]bool
	check   *checkSeen
}

type checkSeen struct {
	brick     string
	index     int
	goesAhead bool
}

// Validate checks a stack top to bottom: brick types, settings, pills, the
// fields each brick needs, and the Check first rules.
func (c *Catalog) Validate(st Stack, opts Options) []Issue {
	var issues []Issue
	add := func(i Issue) { issues = append(issues, i) }

	if st.Schema != 1 {
		add(Issue{Index: -1, Level: LevelError, Code: CodeBadSchema, Message: fmt.Sprintf("stack schema %d is not supported", st.Schema)})
	}
	if len(st.Bricks) == 0 {
		add(Issue{Index: -1, Level: LevelError, Code: CodeEmptyStack, Message: "the stack has no bricks"})
		return issues
	}
	issues = append(issues, validateLimits(st.Limits)...)

	seen := map[string]bool{}
	s := state{fields: map[string]bool{}, context: map[string]bool{}}
	for i, sb := range st.Bricks {
		at := func(level, code, msg string) Issue {
			return Issue{Brick: sb.ID, Index: i, Level: level, Code: code, Message: msg}
		}
		if sb.ID == "" || seen[sb.ID] {
			add(at(LevelError, CodeDuplicateID, fmt.Sprintf("brick id %q is empty or used twice", sb.ID)))
		}
		seen[sb.ID] = true

		b, ok := c.Brick(sb.Type)
		if !ok {
			add(at(LevelError, CodeUnknownBrick, fmt.Sprintf("unknown brick %q", sb.Type)))
			continue
		}
		if b.Status != "mvp" {
			add(at(LevelError, CodeNotAvailable, fmt.Sprintf("%s is not available yet", b.Title.En)))
		}
		switch {
		case i == 0 && b.Group != "when":
			iss := at(LevelError, CodeStartFirst, "a helper starts with a When brick")
			iss.Fix = &IssueFix{Insert: "when.tap", Before: sb.ID}
			add(iss)
		case i > 0 && b.Group == "when":
			add(at(LevelError, CodeStartOnlyFirst, "a When brick can only sit on top"))
		}

		cfg := b.Config(sb.Config)
		issues = append(issues, c.validateSettings(b, sb, i, cfg, opts)...)
		issues = append(issues, c.validateNeeds(b, sb, i, cfg, &s)...)
		issues = append(issues, c.validateCheck(b, sb, i, cfg, s)...)

		// Apply the brick's output to the state.
		adds, open := b.AddsFor(cfg)
		switch b.ShapeFor(cfg) {
		case ShapeStart, ShapeExpand, ShapeCombine:
			s.fields, s.open = map[string]bool{}, open
		default:
			s.open = s.open || open
		}
		for _, f := range adds {
			s.fields[f] = true
		}
		for _, k := range b.providesFor(cfg) {
			s.context[k] = true
		}
		if b.ID == "ask.check" {
			s.check = &checkSeen{brick: sb.ID, index: i, goesAhead: cfg["on_timeout"] == "go"}
		}
	}
	return issues
}

// AddsFor returns the fields the brick adds with this config, and whether
// unknown fields may appear too (e.g. a List's columns).
func (b *Brick) AddsFor(cfg map[string]any) (fields []string, open bool) {
	fields = slices.Clone(b.Adds)
	for _, r := range b.AddsIf {
		if r.If.Matches(cfg) {
			fields = append(fields, r.Fields...)
			open = open || r.Open
		}
	}
	if b.AddsFrom != "" {
		fields = append(fields, stringList(cfg[b.AddsFrom])...)
	}
	return fields, open
}

func (b *Brick) providesFor(cfg map[string]any) []string {
	keys := slices.Clone(b.Provides)
	for _, r := range b.ProvidesIf {
		if r.If.Matches(cfg) {
			keys = append(keys, r.Keys...)
		}
	}
	return keys
}

func (c *Catalog) validateSettings(b *Brick, sb StackBrick, i int, cfg map[string]any, opts Options) []Issue {
	var issues []Issue
	bad := func(key, code, msg string) {
		issues = append(issues, Issue{Brick: sb.ID, Index: i, Level: LevelError, Code: code, Setting: key, Message: msg})
	}
	for key := range sb.Config {
		if _, ok := b.Setting(key); !ok {
			bad(key, CodeUnknownSetting, fmt.Sprintf("%s has no setting %q", b.Title.En, key))
		}
	}
	for idx := range b.Settings {
		set := &b.Settings[idx]
		if !set.Visible(cfg) {
			continue
		}
		v, present := cfg[set.Key]
		if !present || isEmpty(v) {
			if set.Required && !opts.Pending[sb.ID+"."+set.Key] {
				bad(set.Key, CodeMissingSetting, fmt.Sprintf("%s: %q is required", b.Title.En, set.Label.En))
			}
			continue
		}
		if msg := checkValue(c, set, v); msg != "" {
			bad(set.Key, CodeBadValue, fmt.Sprintf("%s: %q %s", b.Title.En, set.Label.En, msg))
		}
	}
	return issues
}

func checkValue(c *Catalog, set *Setting, v any) string {
	switch set.Type {
	case TypeText, TypeLongtext, TypeURL, TypeEmail, TypeTime, TypePlace, TypeList:
		s, ok := v.(string)
		if !ok {
			return "must be text"
		}
		if set.MaxLength > 0 && len([]rune(s)) > set.MaxLength {
			return fmt.Sprintf("is longer than %d characters", set.MaxLength)
		}
		switch set.Type {
		case TypeTime:
			if !timeRe.MatchString(s) {
				return "must be a time like 08:00"
			}
		case TypeEmail:
			if len(Pills(s)) == 0 {
				if a, err := mail.ParseAddress(s); err != nil || a.Address != s {
					return "must be an email address"
				}
			}
		case TypeURL:
			if u, err := url.Parse(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return "must be a web address (https://…)"
			}
		}
	case TypeSelect:
		i := slices.IndexFunc(set.Options, func(o Option) bool { return sameValue(o.Value, v) })
		if i < 0 {
			return fmt.Sprintf("has no option %v", v)
		}
		if set.Options[i].Status == "later" {
			return fmt.Sprintf("option %v is not available yet", v)
		}
	case TypeNumber, TypeSlider:
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) {
			return "must be a number"
		}
		if (set.Min != nil && n < *set.Min) || (set.Max != nil && n > *set.Max) {
			return fmt.Sprintf("must be between %v and %v", deref(set.Min), deref(set.Max))
		}
	case TypeToggle:
		if _, ok := v.(bool); !ok {
			return "must be on or off"
		}
	case TypeDays:
		days, ok := strictStringList(v)
		if !ok || len(days) == 0 {
			return "needs at least one day"
		}
		for _, d := range days {
			if !slices.Contains(weekdays, d) {
				return fmt.Sprintf("has an unknown day %q", d)
			}
		}
	case TypeTags:
		if _, ok := strictStringList(v); !ok {
			return "must be a list of words"
		}
	case TypeFields:
		keys, ok := strictStringList(v)
		if !ok || len(keys) == 0 {
			return "needs at least one detail"
		}
		for _, k := range keys {
			if !keyRe.MatchString(k) {
				return fmt.Sprintf("has a bad detail name %q (use lower_case)", k)
			}
		}
	}
	return ""
}

func (c *Catalog) validateNeeds(b *Brick, sb StackBrick, i int, cfg map[string]any, s *state) []Issue {
	var issues []Issue
	fields := slices.Clone(b.Needs)
	var context []string
	for _, r := range b.NeedsIf {
		if r.If.Matches(cfg) {
			fields = append(fields, r.Fields...)
			context = append(context, r.Context...)
		}
	}
	for idx := range b.Settings {
		set := &b.Settings[idx]
		text, isText := cfg[set.Key].(string)
		if !set.Pills || !set.Visible(cfg) || !isText {
			continue
		}
		for _, p := range Pills(text) {
			switch p.Kind {
			case "field":
				fields = append(fields, p.Key)
			case "context":
				context = append(context, p.Key)
			case "profile":
				if !c.HasProfile(p.Key) {
					issues = append(issues, Issue{Brick: sb.ID, Index: i, Level: LevelError, Code: CodeUnknownProfile,
						Setting: set.Key, Field: p.Key, Message: fmt.Sprintf("{%s}: there is no About me fact %q", p.Raw, p.Key)})
				}
			default:
				issues = append(issues, Issue{Brick: sb.ID, Index: i, Level: LevelError, Code: CodeBadPill,
					Setting: set.Key, Message: fmt.Sprintf("{%s} is not a valid placeholder", p.Raw)})
			}
		}
	}

	reported := map[string]bool{}
	for _, f := range fields {
		if s.fields[f] || s.open || reported[f] {
			continue
		}
		reported[f] = true
		iss := Issue{Brick: sb.ID, Index: i, Level: LevelError, Code: CodeMissingField, Field: f,
			Message: fmt.Sprintf("%s needs {%s}, but no brick above provides it", b.Title.En, f)}
		if fix, ok := c.Fixes[f]; ok {
			iss.Fix = &IssueFix{Insert: fix.Brick, Config: fix.Config, Before: sb.ID}
			if fb, ok := c.Brick(fix.Brick); ok {
				iss.Message += fmt.Sprintf(". Add %q first?", fb.Title.En)
			}
		}
		issues = append(issues, iss)
	}
	for _, k := range context {
		if s.context[k] || reported["ctx:"+k] {
			continue
		}
		reported["ctx:"+k] = true
		msg := fmt.Sprintf("%s uses {%s}, but nothing above provides it", b.Title.En, k)
		switch {
		case k == "answer":
			msg += ` (add "Ask me something", or a question to "Tap Go")`
		case strings.HasPrefix(k, "trigger."):
			msg += ` (only available when the helper starts with "Email this helper")`
		}
		issues = append(issues, Issue{Brick: sb.ID, Index: i, Level: LevelError, Code: CodeMissingContext, Field: k, Message: msg})
	}
	return issues
}

func (c *Catalog) validateCheck(b *Brick, sb StackBrick, i int, cfg map[string]any, s state) []Issue {
	if b.Check == nil {
		return nil
	}
	required := len(b.Check.RequiredIf) > 0 && b.Check.RequiredIf.Matches(cfg)
	fix := func() *IssueFix {
		f, ok := c.Fixes["check"]
		if !ok {
			return nil
		}
		return &IssueFix{Insert: f.Brick, Config: f.Config, Before: sb.ID}
	}
	switch {
	case required && s.check == nil:
		return []Issue{{Brick: sb.ID, Index: i, Level: LevelError, Code: CodeCheckRequired, Fix: fix(),
			Message: fmt.Sprintf("%s goes to other people, so it needs \"Check first\" above it", b.Title.En)}}
	case required && s.check.goesAhead:
		return []Issue{{Brick: s.check.brick, Index: s.check.index, Level: LevelError, Code: CodeCheckGoesAhead, Setting: "on_timeout",
			Message: fmt.Sprintf("\"Check first\" must skip, not go ahead, when you don't answer: %s goes to other people", b.Title.En)}}
	case b.Check.Default && s.check == nil:
		return []Issue{{Brick: sb.ID, Index: i, Level: LevelHint, Code: CodeCheckSuggested, Fix: fix(),
			Message: fmt.Sprintf("%s leaves Laamo. Check with you first?", b.Title.En)}}
	}
	return nil
}

func validateLimits(l *Limits) []Issue {
	if l == nil {
		return nil
	}
	var issues []Issue
	bad := func(key, msg string) {
		issues = append(issues, Issue{Index: -1, Level: LevelError, Code: CodeBadLimit, Setting: key, Message: msg})
	}
	if l.MaxCards != nil && (*l.MaxCards < 1 || *l.MaxCards > 500) {
		bad("max_cards", "max_cards must be between 1 and 500")
	}
	if l.MaxExternalEmails != nil && (*l.MaxExternalEmails < 0 || *l.MaxExternalEmails > 100) {
		bad("max_external_emails", "max_external_emails must be between 0 and 100")
	}
	if l.MaxCostEUR != nil && (*l.MaxCostEUR <= 0 || *l.MaxCostEUR > 10) {
		bad("max_cost_eur", "max_cost_eur must be above 0 and at most 10")
	}
	return issues
}

func deref(f *float64) any {
	if f == nil {
		return "∞"
	}
	return *f
}

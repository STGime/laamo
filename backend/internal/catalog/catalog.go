// Package catalog loads the brick catalog (packages/catalog/bricks.json) and
// the Shelf templates, and validates stacks against them.
//
// The same file drives the web builder; see packages/catalog/README.md for
// the format.
package catalog

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sync"

	files "github.com/STGime/laamo/packages/catalog"
)

// Text is a localized string.
type Text struct {
	En string `json:"en"`
	De string `json:"de"`
}

// In returns the text for lang ("en" or "de"), falling back to English.
func (t Text) In(lang string) string {
	if lang == "de" && t.De != "" {
		return t.De
	}
	return t.En
}

type Catalog struct {
	Schema  int            `json:"schema"`
	Groups  []Group        `json:"groups"`
	Profile []ProfileFact  `json:"profile"`
	Fields  []Field        `json:"fields"`
	Fixes   map[string]Fix `json:"fixes"`
	Bricks  []Brick        `json:"bricks"`

	byID map[string]*Brick
}

type Group struct {
	ID       string `json:"id"`
	Color    string `json:"color"`
	Title    Text   `json:"title"`
	Question Text   `json:"question"`
}

type ProfileFact struct {
	Key      string `json:"key"`
	Label    Text   `json:"label"`
	Question Text   `json:"question"`
}

type Field struct {
	Key   string `json:"key"`
	Label Text   `json:"label"`
	Pick  bool   `json:"pick,omitempty"`
}

// Fix is a one-tap fix: insert this brick (with config) before the brick
// that has the problem.
type Fix struct {
	Brick  string         `json:"brick"`
	Config map[string]any `json:"config,omitempty"`
}

// Shapes: how a brick turns its input pile into its output pile.
const (
	ShapeStart   = "start"   // nothing → 0 or 1 card
	ShapeExpand  = "expand"  // 1 → many new cards
	ShapeEnrich  = "enrich"  // each card → same card, more fields
	ShapeFilter  = "filter"  // many → fewer (or the same)
	ShapeCombine = "combine" // many → 1 new card
	ShapeAct     = "act"     // pile → same pile, marked done
)

type Brick struct {
	ID         string         `json:"id"`
	Group      string         `json:"group"`
	Shape      string         `json:"shape"`
	ShapeIf    []ShapeRule    `json:"shape_if,omitempty"`
	Status     string         `json:"status"` // mvp, later
	Trigger    string         `json:"trigger,omitempty"`
	Title      Text           `json:"title"`
	Does       Text           `json:"does"`
	Sentence   []SentenceRule `json:"sentence"`
	Settings   []Setting      `json:"settings"`
	CardKind   string         `json:"card_kind,omitempty"`
	Needs      []string       `json:"needs,omitempty"`
	NeedsIf    []NeedsRule    `json:"needs_if,omitempty"`
	Adds       []string       `json:"adds,omitempty"`
	AddsIf     []AddsRule     `json:"adds_if,omitempty"`
	AddsFrom   string         `json:"adds_from,omitempty"`
	Provides   []string       `json:"provides,omitempty"`
	ProvidesIf []ProvidesRule `json:"provides_if,omitempty"`
	Requires   *Requires      `json:"requires,omitempty"`
	RequiresIf []RequiresRule `json:"requires_if,omitempty"`
	Check      *CheckRule     `json:"check,omitempty"`
}

type ShapeRule struct {
	If    Condition `json:"if"`
	Shape string    `json:"shape"`
}

type SentenceRule struct {
	If   Condition `json:"if,omitempty"`
	Text Text      `json:"text"`
}

type NeedsRule struct {
	If      Condition `json:"if"`
	Fields  []string  `json:"fields,omitempty"`
	Context []string  `json:"context,omitempty"`
}

type AddsRule struct {
	If     Condition `json:"if"`
	Fields []string  `json:"fields,omitempty"`
	Open   bool      `json:"open,omitempty"`
}

type ProvidesRule struct {
	If   Condition `json:"if"`
	Keys []string  `json:"keys"`
}

type Requires struct {
	Brain  bool     `json:"brain,omitempty"`
	Places []string `json:"places,omitempty"` // any one of these place types
	Plan   string   `json:"plan,omitempty"`   // "paid"
}

type RequiresRule struct {
	If Condition `json:"if"`
	Requires
}

type CheckRule struct {
	Default    bool      `json:"default,omitempty"`
	RequiredIf Condition `json:"required_if,omitempty"`
}

// Setting types.
const (
	TypeText     = "text"
	TypeLongtext = "longtext"
	TypeURL      = "url"
	TypeEmail    = "email"
	TypeSelect   = "select"
	TypeDays     = "days"
	TypeTime     = "time"
	TypeNumber   = "number"
	TypeSlider   = "slider"
	TypeToggle   = "toggle"
	TypeTags     = "tags"
	TypeFields   = "fields"
	TypePlace    = "place"
	TypeList     = "list"
)

type Setting struct {
	Key         string    `json:"key"`
	Type        string    `json:"type"`
	Label       Text      `json:"label"`
	Help        *Text     `json:"help,omitempty"`
	Placeholder *Text     `json:"placeholder,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Advanced    bool      `json:"advanced,omitempty"`
	Default     any       `json:"default,omitempty"`
	ShowIf      Condition `json:"show_if,omitempty"`
	Pills       bool      `json:"pills,omitempty"`
	Options     []Option  `json:"options,omitempty"`
	Min         *float64  `json:"min,omitempty"`
	Max         *float64  `json:"max,omitempty"`
	Step        float64   `json:"step,omitempty"`
	MaxLength   int       `json:"max_length,omitempty"`
	Ends        []Text    `json:"ends,omitempty"`
	PlaceTypes  []string  `json:"place_types,omitempty"`
	Create      bool      `json:"create,omitempty"`
}

type Option struct {
	Value  any    `json:"value"`
	Label  Text   `json:"label"`
	Status string `json:"status,omitempty"`
}

// Condition matches a brick config: every key must match. A value is either
// the string "set" (non-empty) or a list of allowed values.
type Condition map[string]json.RawMessage

// Matches reports whether cfg (with defaults applied) satisfies c.
// An empty condition always matches.
func (c Condition) Matches(cfg map[string]any) bool {
	for key, raw := range c {
		var set string
		if json.Unmarshal(raw, &set) == nil && set == "set" {
			if isEmpty(cfg[key]) {
				return false
			}
			continue
		}
		var allowed []any
		if json.Unmarshal(raw, &allowed) != nil {
			return false
		}
		if !slices.ContainsFunc(allowed, func(a any) bool { return sameValue(a, cfg[key]) }) {
			return false
		}
	}
	return true
}

// ---- loading ----

// Load parses a catalog from JSON.
func Load(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if c.Schema != 1 {
		return nil, fmt.Errorf("catalog: unsupported schema %d", c.Schema)
	}
	c.byID = make(map[string]*Brick, len(c.Bricks))
	for i := range c.Bricks {
		b := &c.Bricks[i]
		if _, dup := c.byID[b.ID]; dup {
			return nil, fmt.Errorf("catalog: duplicate brick %s", b.ID)
		}
		c.byID[b.ID] = b
	}
	return &c, nil
}

var (
	defaultOnce sync.Once
	defaultCat  *Catalog
	defaultErr  error
)

// Default returns the embedded catalog. It panics if the embedded file is
// invalid, which the tests rule out.
func Default() *Catalog {
	defaultOnce.Do(func() {
		data, err := fs.ReadFile(files.Files, "bricks.json")
		if err != nil {
			defaultErr = err
			return
		}
		defaultCat, defaultErr = Load(data)
	})
	if defaultErr != nil {
		panic(defaultErr)
	}
	return defaultCat
}

// Brick returns the catalog entry for a brick type.
func (c *Catalog) Brick(id string) (*Brick, bool) {
	b, ok := c.byID[id]
	return b, ok
}

// Group returns a group by id.
func (c *Catalog) Group(id string) (Group, bool) {
	for _, g := range c.Groups {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

// HasProfile reports whether key is an About me fact.
func (c *Catalog) HasProfile(key string) bool {
	return slices.ContainsFunc(c.Profile, func(p ProfileFact) bool { return p.Key == key })
}

// Setting returns a brick's setting by key.
func (b *Brick) Setting(key string) (*Setting, bool) {
	for i := range b.Settings {
		if b.Settings[i].Key == key {
			return &b.Settings[i], true
		}
	}
	return nil, false
}

// Config returns cfg with defaults filled in for missing settings, and
// values normalised to JSON types (float64 numbers).
func (b *Brick) Config(cfg map[string]any) map[string]any {
	out := normalize(cfg)
	for _, s := range b.Settings {
		if _, ok := out[s.Key]; !ok && s.Default != nil {
			out[s.Key] = normalizeValue(s.Default)
		}
	}
	return out
}

// ShapeFor returns the brick's shape for a config (shape_if overrides).
func (b *Brick) ShapeFor(cfg map[string]any) string {
	for _, r := range b.ShapeIf {
		if r.If.Matches(cfg) {
			return r.Shape
		}
	}
	return b.Shape
}

// Visible reports whether a setting is shown for this config.
func (s *Setting) Visible(cfg map[string]any) bool { return s.ShowIf.Matches(cfg) }

// ---- templates ----

type Template struct {
	Schema         int             `json:"schema"`
	Slug           string          `json:"slug"`
	Name           Text            `json:"name"`
	Promise        Text            `json:"promise"`
	Category       string          `json:"category"`
	Avatar         string          `json:"avatar"`
	SortOrder      int             `json:"sort_order,omitempty"`
	Notice         *Text           `json:"notice,omitempty"`
	SetupQuestions []SetupQuestion `json:"setup_questions"`
	Stack          Stack           `json:"stack"`
	Required       Requires        `json:"required"`
}

// SetupQuestion either asks for an About me fact (Profile) or fills brick
// settings for this helper (Fills).
type SetupQuestion struct {
	Profile     string `json:"profile,omitempty"`
	Key         string `json:"key,omitempty"`
	Type        string `json:"type,omitempty"`
	Question    *Text  `json:"question,omitempty"`
	Placeholder *Text  `json:"placeholder,omitempty"`
	Fills       []struct {
		Brick   string `json:"brick"`
		Setting string `json:"setting"`
	} `json:"fills,omitempty"`
}

// Templates returns the embedded Shelf templates, by sort order.
func Templates() ([]Template, error) {
	names, err := fs.Glob(files.Files, "templates/*.json")
	if err != nil {
		return nil, err
	}
	var out []Template
	for _, name := range names {
		data, err := fs.ReadFile(files.Files, name)
		if err != nil {
			return nil, err
		}
		var t Template
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("catalog: %s: %w", path.Base(name), err)
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Template) int { return a.SortOrder - b.SortOrder })
	return out, nil
}

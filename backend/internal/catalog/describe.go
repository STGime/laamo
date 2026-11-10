package catalog

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Needs is what a stack requires before it can run.
type Needs struct {
	Brain bool `json:"brain"`
	Paid  bool `json:"paid"`
	// Places: each entry is a set of place types of which one is needed,
	// e.g. [[discord slack teams telegram]] for Post to chat.
	Places [][]string `json:"places,omitempty"`
}

// Requirements collects brain, plan and place needs of a stack.
func (c *Catalog) Requirements(st Stack) Needs {
	var n Needs
	addPlaces := func(p []string) {
		if len(p) == 0 {
			return
		}
		sorted := slices.Sorted(slices.Values(p))
		if !slices.ContainsFunc(n.Places, func(q []string) bool { return slices.Equal(q, sorted) }) {
			n.Places = append(n.Places, sorted)
		}
	}
	apply := func(r Requires) {
		n.Brain = n.Brain || r.Brain
		n.Paid = n.Paid || r.Plan == "paid"
		addPlaces(r.Places)
	}
	for _, sb := range st.Bricks {
		b, ok := c.Brick(sb.Type)
		if !ok {
			continue
		}
		cfg := b.Config(sb.Config)
		if b.Requires != nil {
			apply(*b.Requires)
		}
		for _, r := range b.RequiresIf {
			if r.If.Matches(cfg) {
				apply(r.Requires)
			}
		}
	}
	return n
}

// Sentence renders a brick as one plain sentence in lang, e.g.
// "Find bakeries in {my city}". Pills stay as written; the UI shows them as
// chips. Place and list ids are shown as is (the UI resolves their names).
func (c *Catalog) Sentence(sb StackBrick, lang string) string {
	b, ok := c.Brick(sb.Type)
	if !ok {
		return sb.Type
	}
	cfg := b.Config(sb.Config)
	text := b.Title.In(lang)
	for _, r := range b.Sentence {
		if r.If.Matches(cfg) {
			text = r.Text.In(lang)
			break
		}
	}
	for idx := range b.Settings {
		set := &b.Settings[idx]
		text = strings.ReplaceAll(text, "["+set.Key+"]", c.display(set, cfg[set.Key], lang))
	}
	return text
}

var dayNames = map[string]Text{
	"mon": {"Monday", "Montag"}, "tue": {"Tuesday", "Dienstag"}, "wed": {"Wednesday", "Mittwoch"},
	"thu": {"Thursday", "Donnerstag"}, "fri": {"Friday", "Freitag"}, "sat": {"Saturday", "Samstag"},
	"sun": {"Sunday", "Sonntag"},
}

func (c *Catalog) display(set *Setting, v any, lang string) string {
	and := map[string]string{"en": " and ", "de": " und "}[lang]
	if and == "" {
		and = " and "
	}
	join := func(items []string) string {
		switch len(items) {
		case 0:
			return ""
		case 1:
			return items[0]
		}
		return strings.Join(items[:len(items)-1], ", ") + and + items[len(items)-1]
	}
	switch set.Type {
	case TypeSelect:
		for _, o := range set.Options {
			if sameValue(o.Value, v) {
				return o.Label.In(lang)
			}
		}
	case TypeDays:
		var names []string
		for _, d := range stringList(v) {
			if t, ok := dayNames[d]; ok {
				names = append(names, t.In(lang))
			}
		}
		return join(names)
	case TypeFields:
		var names []string
		for _, k := range stringList(v) {
			label := k
			if i := slices.IndexFunc(c.Fields, func(f Field) bool { return f.Key == k }); i >= 0 {
				label = c.Fields[i].Label.In(lang)
			}
			names = append(names, label)
		}
		return join(names)
	case TypeTags:
		return join(stringList(v))
	case TypeNumber, TypeSlider:
		if f, ok := v.(float64); ok {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

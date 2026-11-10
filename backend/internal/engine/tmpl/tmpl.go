// Package tmpl fills in the pills in brick settings, per card:
//
//	{email}, {name}, …        → the card's fields
//	{my city}, {my offer}, …  → the About me profile
//	{answer}, {trigger.subject} → run values (Ask me something, Tap Go,
//	                              the email that started the run)
//
// A missing value is an error that says what's missing; it never turns into
// an empty string, so a helper can't send "Hi ," or search for "bakeries in ".
package tmpl

import (
	"fmt"
	"strings"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
)

// ErrMissing is wrapped by every "value is missing" error. It is
// engine.ErrMissingValue, so the engine ends the run with a Problem card.
var ErrMissing = engine.ErrMissingValue

// MissingError says which pill had no value.
type MissingError struct {
	Pill catalog.Pill
	Card string // card id, for field pills
}

func (e *MissingError) Error() string {
	switch e.Pill.Kind {
	case "profile":
		return fmt.Sprintf("{%s} isn't set yet. Add it under Me → About me", e.Pill.Raw)
	case "context":
		return fmt.Sprintf("{%s} has no value in this run", e.Pill.Raw)
	case "field":
		return fmt.Sprintf("a card has no {%s}", e.Pill.Raw)
	}
	return fmt.Sprintf("{%s} is not a valid placeholder", e.Pill.Raw)
}

func (e *MissingError) Unwrap() error { return ErrMissing }

// Source looks up pill values.
type Source interface {
	Field(key string) (string, bool)
	Profile(key string) (string, bool)
	Value(key string) (string, bool)
}

// Resolve replaces every {pill} in s using src.
func Resolve(s string, src Source) (string, error) {
	var firstErr error
	out := pillRe(s, func(raw string) string {
		ps := catalog.Pills("{" + raw + "}")
		if len(ps) != 1 {
			return "{" + raw + "}"
		}
		p := ps[0]
		var v string
		var ok bool
		switch p.Kind {
		case "field":
			v, ok = src.Field(p.Key)
		case "profile":
			v, ok = src.Profile(p.Key)
		case "context":
			v, ok = src.Value(p.Key)
		}
		v = strings.TrimSpace(v)
		if !ok || v == "" {
			if firstErr == nil {
				firstErr = &MissingError{Pill: p}
			}
			return ""
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// pillRe calls f for each {…} in s and replaces it with the result.
func pillRe(s string, f func(raw string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i+1:], '}')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		inner := s[i+1 : i+1+j]
		if strings.ContainsRune(inner, '{') { // "{{x}": keep the first brace literally
			b.WriteString(s[:i+1])
			s = s[i+1:]
			continue
		}
		b.WriteString(s[:i])
		b.WriteString(f(inner))
		s = s[i+1+j+1:]
	}
}

// For returns the Source for one card in a run. card may be nil when a
// setting is resolved once for the whole run (e.g. a search query).
func For(rc *engine.RunContext, card *engine.Card) Source {
	return source{rc: rc, card: card}
}

type source struct {
	rc   *engine.RunContext
	card *engine.Card
}

func (s source) Field(key string) (string, bool) {
	if s.card == nil {
		return "", false
	}
	v := s.card.Text(key)
	return v, v != ""
}

func (s source) Profile(key string) (string, bool) { return s.rc.Profile(key) }
func (s source) Value(key string) (string, bool)   { return s.rc.Value(key) }

// Config returns cfg with the pills of every visible pill setting of brick b
// resolved for card (nil = run-wide). Settings without pills are unchanged.
func Config(rc *engine.RunContext, b *catalog.Brick, cfg engine.Config, card *engine.Card) (engine.Config, error) {
	out := make(engine.Config, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	src := For(rc, card)
	for i := range b.Settings {
		set := &b.Settings[i]
		text, isText := cfg[set.Key].(string)
		if !set.Pills || !isText || !set.Visible(cfg) {
			continue
		}
		resolved, err := Resolve(text, src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", set.Label.En, err)
		}
		out[set.Key] = resolved
	}
	return out, nil
}

package tmpl_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/store"
)

func runCtx() *engine.RunContext {
	rc := &engine.RunContext{Workspace: store.Workspace{Profile: map[string]any{"city": "Leipzig", "offer": "  ", "business": "bakery"}}}
	rc.SetValue("answer", "Dresden")
	rc.SetValue("trigger.subject", "Can we meet on Tuesday?")
	return rc
}

func card() *engine.Card {
	c := &engine.Card{ID: "c1"}
	c.Set("name", "Bäckerei Maier", engine.TrustExtracted)
	c.Set("email", "info@maier.de", engine.TrustExtracted)
	c.Set("price", 4.5, engine.TrustExtracted)
	c.Set("empty", "   ", engine.TrustExtracted)
	return c
}

func TestResolve(t *testing.T) {
	src := tmpl.For(runCtx(), card())
	cases := []struct{ in, want string }{
		{"Hi {name}, from {my city}", "Hi Bäckerei Maier, from Leipzig"},
		{"bakeries in {answer}", "bakeries in Dresden"},
		{"Re: {trigger.subject}", "Re: Can we meet on Tuesday?"},
		{"costs {price} €", "costs 4.5 €"},
		{"no pills here", "no pills here"},
		{"a } b { c", "a } b { c"},
		{"{{name}}", "{Bäckerei Maier}"},
		{"{ my business }", "bakery"},
	}
	for _, tc := range cases {
		got, err := tmpl.Resolve(tc.in, src)
		if err != nil || got != tc.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestMissingValues(t *testing.T) {
	src := tmpl.For(runCtx(), card())
	cases := []struct{ in, kind, says string }{
		{"Hi {phone}", "field", "a card has no {phone}"},
		{"Hi {empty}", "field", "{empty}"},
		{"from {my website}", "profile", "Me → About me"},
		{"offering {my offer}", "profile", "{my offer} isn't set yet"},
		{"re {trigger.from}", "context", "{trigger.from} has no value"},
		{"what { }", "", "not a valid placeholder"},
	}
	for _, tc := range cases {
		got, err := tmpl.Resolve(tc.in, src)
		var me *tmpl.MissingError
		if got != "" || !errors.As(err, &me) || me.Pill.Kind != tc.kind || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("Resolve(%q) = %q, %v", tc.in, got, err)
		}
		if !errors.Is(err, engine.ErrMissingValue) {
			t.Errorf("%q: the engine must see ErrMissingValue", tc.in)
		}
	}
	// A run-wide setting can't use card fields.
	if _, err := tmpl.Resolve("{name}", tmpl.For(runCtx(), nil)); !errors.Is(err, tmpl.ErrMissing) {
		t.Errorf("field pill without a card: %v", err)
	}
}

func TestConfig(t *testing.T) {
	cat := catalog.Default()
	write, _ := cat.Brick("think.write")
	cfg := engine.Config(write.Config(map[string]any{"what": "an intro to {name} offering bread from {my city}"}))
	got, err := tmpl.Config(runCtx(), write, cfg, card())
	if err != nil {
		t.Fatal(err)
	}
	if got.String("what") != "an intro to Bäckerei Maier offering bread from Leipzig" || got.String("mode") != "each" {
		t.Fatalf("config: %v", got)
	}
	if cfg.String("what") == got.String("what") {
		t.Fatal("the input config must not change")
	}

	// Only pill settings are resolved, and only visible ones.
	email, _ := cat.Brick("do.email")
	cfg = engine.Config(email.Config(map[string]any{"to": "me", "address": "{nope}", "subject": "Leads for {my city}"}))
	got, err = tmpl.Config(runCtx(), email, cfg, nil)
	if err != nil || got.String("subject") != "Leads for Leipzig" || got.String("address") != "{nope}" {
		t.Fatalf("config: %v %v", got, err)
	}

	_, err = tmpl.Config(runCtx(), write, engine.Config(write.Config(map[string]any{"what": "hi {phone}"})), card())
	if !errors.Is(err, tmpl.ErrMissing) || !strings.HasPrefix(err.Error(), "What to write: ") {
		t.Fatalf("missing value names the setting: %v", err)
	}
}

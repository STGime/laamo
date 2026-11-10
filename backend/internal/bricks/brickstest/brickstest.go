// Package brickstest has the contract test every brick executor runs: given
// a pile and a config, the output has the brick's catalog shape and only
// the fields the catalog says it adds (plus the input's own fields).
package brickstest

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
)

// RunContext returns a run context for brick tests: a workspace with a few
// About me facts, default limits, a quiet logger.
func RunContext(brickType string) *engine.RunContext {
	b, _ := catalog.Default().Brick(brickType)
	return &engine.RunContext{
		Run:       store.Run{ID: store.NewID(), WorkspaceID: store.NewID(), UserID: store.NewID()},
		Workspace: store.Workspace{Timezone: "Europe/Berlin", Locale: "en", Profile: map[string]any{"city": "Leipzig", "business": "bakery"}},
		Brick:     b,
		BrickID:   "b1",
		Limits:    engine.DefaultLimits,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:       func() time.Time { return time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC) },
	}
}

// Contract runs brick b of type brickType on in with cfg (catalog defaults
// filled in) and checks its output. It returns the output for further checks.
func Contract(t *testing.T, brickType string, b engine.Brick, rc *engine.RunContext, in []engine.Card, cfg map[string]any) ([]engine.Card, error) {
	t.Helper()
	cb, ok := catalog.Default().Brick(brickType)
	if !ok {
		t.Fatalf("no catalog entry %s", brickType)
	}
	if rc == nil {
		rc = RunContext(brickType)
	}
	full := cb.Config(cfg)
	out, err := b.Run(context.Background(), rc, clone(in), engine.Config(full))
	if err != nil {
		return nil, err
	}
	shape := cb.ShapeFor(full)
	out, err = engine.CheckShape(shape, in, out)
	if err != nil {
		t.Fatalf("%s: %v", brickType, err)
	}

	adds, open := cb.AddsFor(full)
	allowed := map[string]bool{}
	for _, f := range adds {
		allowed[f] = true
	}
	inFields := map[string]bool{}
	for _, c := range in {
		for k := range c.Fields {
			inFields[k] = true
		}
	}
	keepsFields := shape != catalog.ShapeExpand && shape != catalog.ShapeCombine && shape != catalog.ShapeStart
	for i, c := range out {
		for _, k := range slices.Sorted(maps.Keys(c.Fields)) {
			if open || allowed[k] || (keepsFields && inFields[k]) {
				continue
			}
			t.Errorf("%s: card %d has field %q, which the catalog doesn't declare in adds", brickType, i, k)
		}
		for k := range c.Trust {
			if _, ok := c.Fields[k]; !ok {
				t.Errorf("%s: card %d has trust for missing field %q", brickType, i, k)
			}
		}
		for k := range c.Fields {
			if _, ok := c.Trust[k]; !ok && !inFields[k] {
				t.Errorf("%s: card %d sets %q without a trust level", brickType, i, k)
			}
		}
	}
	if len(out) > 0 && !open {
		for _, f := range adds {
			if !slices.ContainsFunc(out, func(c engine.Card) bool { _, ok := c.Fields[f]; return ok }) {
				t.Errorf("%s: no output card has %q, which the catalog says it adds", brickType, f)
			}
		}
	}
	return out, nil
}

func clone(in []engine.Card) []engine.Card {
	out := make([]engine.Card, len(in))
	for i, c := range in {
		out[i] = c.Clone()
	}
	return out
}

package think

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
)

// KeepThreshold: cards scoring below this are dropped.
const KeepThreshold = 5

// Keep is the Keep the good ones brick (think.keep): scores every card 0–10
// against the user's words, keeps the good ones (best first, or in their
// order), at most max.
var Keep = engine.BrickFunc(func(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if rc.Brain == nil {
		return nil, queue.Permanent(errors.New("the AI isn't set up"))
	}
	resolved, err := tmpl.Config(rc, rc.Brick, cfg, nil)
	if err != nil {
		return nil, err
	}
	criteria := strings.TrimSpace(resolved.String("criteria"))
	lang := language(rc, "")

	type score struct {
		ID     string  `json:"id"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	scores := map[string]score{}
	for _, batch := range batches(in, 20) {
		var out struct {
			Cards []score `json:"cards"`
		}
		system := fmt.Sprintf(`You judge cards for a small business owner. Score each card from 0 (useless) to 10 (exactly what they want) by how well it matches what they want. Give a reason of at most 12 words in %s. %s Format: {"cards":[{"id": string, "score": number, "reason": string}]} with one entry for every card id.`, lang, guard)
		user := fmt.Sprintf("What they want: %q\n%s", criteria, cardsJSON(batch))
		ids := idSet(batch)
		err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: user}}}, &out,
			func() error {
				seen := map[string]bool{}
				for _, s := range out.Cards {
					switch {
					case !ids[s.ID]:
						return fmt.Errorf("unknown card id %q", s.ID)
					case seen[s.ID]:
						return fmt.Errorf("card %q scored twice", s.ID)
					case s.Score < 0 || s.Score > 10:
						return fmt.Errorf("score %v for %q is outside 0–10", s.Score, s.ID)
					}
					seen[s.ID] = true
				}
				if len(seen) != len(ids) {
					return fmt.Errorf("score all %d cards, got %d", len(ids), len(seen))
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		for _, s := range out.Cards {
			scores[s.ID] = s
		}
	}

	var kept []engine.Card
	for _, c := range in {
		s := scores[c.ID]
		if s.Score < KeepThreshold {
			continue
		}
		c.Score, c.Reason = new(s.Score), strings.TrimSpace(s.Reason)
		c.Set("score", s.Score, engine.TrustExtracted)
		c.Set("reason", c.Reason, engine.TrustExtracted)
		kept = append(kept, c)
	}
	if cfg.String("sort") != "original" {
		slices.SortStableFunc(kept, func(a, b engine.Card) int { return cmp.Compare(*b.Score, *a.Score) })
	}
	if n := cfg.Int("max"); n > 0 && len(kept) > n {
		kept = kept[:n]
	}
	return kept, nil
})

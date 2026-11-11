package think

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
)

var lengths = map[string]string{
	"line":     "one sentence of at most 20 words",
	"short":    "at most 80 words; a few short bullet points are fine",
	"detailed": "at most 250 words, with short paragraphs or bullet points",
}

// maxSources caps the links a summary keeps.
const maxSources = 10

// Summarize is the Sum it up brick (think.summarize).
var Summarize = engine.BrickFunc(func(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if rc.Brain == nil {
		return nil, queue.Permanent(errors.New("the AI isn't set up"))
	}
	length := lengths[cfg.String("length")]
	if length == "" {
		length = lengths["short"]
	}
	lang := language(rc, cfg.String("language"))

	if cfg.String("mode") == "each" {
		return summarizeEach(ctx, rc, in, length, lang)
	}

	var out struct {
		Summary string `json:"summary"`
	}
	system := fmt.Sprintf(`You summarise cards for a small business owner. Write in %s. Length: %s. Lead with what matters most. %s Format: {"summary": string}.`, lang, length, guard)
	err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: cardsJSON(in)}}}, &out,
		func() error {
			if strings.TrimSpace(out.Summary) == "" {
				return errors.New(`"summary" is empty`)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	var c engine.Card
	c.Kind = "summary"
	c.Set("summary", strings.TrimSpace(out.Summary), engine.TrustExtracted)
	// Where it came from: the summarised cards' titles and links, so people
	// can read the original (and check the summary).
	var sources []any
	for _, card := range in {
		u := card.Text("url")
		if u == "" {
			u = card.Source
		}
		if u == "" || len(sources) == maxSources {
			continue
		}
		sources = append(sources, map[string]any{"title": cut(cmp.Or(card.Text("title"), card.Text("name"), u), 160), "url": u})
	}
	if len(sources) > 0 {
		c.Set("sources", sources, engine.TrustUntrusted)
	}
	return []engine.Card{c}, nil
})

func summarizeEach(ctx context.Context, rc *engine.RunContext, in []engine.Card, length, lang string) ([]engine.Card, error) {
	byID := map[string]string{}
	for _, batch := range batches(in, 10) {
		var out struct {
			Summaries []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
			} `json:"summaries"`
		}
		system := fmt.Sprintf(`You summarise each card separately for a small business owner. Write in %s. Length per card: %s. %s Format: {"summaries":[{"id": string, "summary": string}]} with one entry for every card id.`, lang, length, guard)
		ids := idSet(batch)
		err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: cardsJSON(batch)}}}, &out,
			func() error {
				got := map[string]bool{}
				for _, s := range out.Summaries {
					if !ids[s.ID] || strings.TrimSpace(s.Summary) == "" {
						return fmt.Errorf("entry %q is unknown or empty", s.ID)
					}
					got[s.ID] = true
				}
				if len(got) != len(ids) {
					return fmt.Errorf("give a summary for all %d cards, got %d", len(ids), len(got))
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		for _, s := range out.Summaries {
			byID[s.ID] = strings.TrimSpace(s.Summary)
		}
	}
	for i := range in {
		in[i].Set("summary", byID[in[i].ID], engine.TrustExtracted)
	}
	return in, nil
}

func idSet(cards []engine.Card) map[string]bool {
	ids := make(map[string]bool, len(cards))
	for _, c := range cards {
		ids[c.ID] = true
	}
	return ids
}

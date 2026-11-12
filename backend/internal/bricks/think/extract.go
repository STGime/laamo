package think

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
)

// What each known detail should look like, for the model. Other (custom)
// details are free text.
var detailHints = map[string]string{
	"name":    "the person's or business's name",
	"email":   "an email address",
	"phone":   "a phone number as written",
	"website": "the website address",
	"address": "the postal address on one line",
	"price":   "the price with its currency, e.g. \"12,50 €\"",
	"date":    "the date as YYYY-MM-DD",
	"time":    "the start time as HH:MM (24h)",
	"place":   "where it happens: venue and city",
	"title":   "a short title of at most 10 words",
}

// maxDetailChars caps one pulled-out value.
const maxDetailChars = 300

var (
	emailRe = regexp.MustCompile(`^[^@\s<>"]+@[^@\s<>"]+\.[a-z]{2,}$`)
	dateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRe  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// Extract is the Pull out details brick (think.extract): fills the chosen
// fields on every card. Values are marked extracted. An email, phone or
// website is only kept when it's really in the card: the model must not
// make up a recipient.
var Extract = engine.BrickFunc(func(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if rc.Brain == nil {
		return nil, queue.Permanent(errors.New("the AI isn't set up"))
	}
	fields := cfg.Strings("fields")
	if len(fields) == 0 {
		return in, nil
	}
	var want []string
	for _, f := range fields {
		hint := detailHints[f]
		if hint == "" {
			hint = "free text: " + strings.ReplaceAll(f, "_", " ")
		}
		want = append(want, fmt.Sprintf("%q: %s", f, hint))
	}

	for _, batch := range batches(in, 10) {
		var out struct {
			Cards []struct {
				ID     string         `json:"id"`
				Fields map[string]any `json:"fields"`
			} `json:"cards"`
		}
		system := fmt.Sprintf(`You pull details out of cards for a small business owner. Details to pull out:
%s
Use only what the card itself says. If a detail isn't in the card, use null: never guess or make one up. %s Format: {"cards":[{"id": string, "fields": {detail: string or null}}]} with one entry for every card id.`,
			"- "+strings.Join(want, "\n- "), guard)
		ids := idSet(batch)
		err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: cardsJSON(batch)}}}, &out,
			func() error {
				got := map[string]bool{}
				for _, c := range out.Cards {
					if !ids[c.ID] || got[c.ID] {
						return fmt.Errorf("card %q is unknown or listed twice", c.ID)
					}
					got[c.ID] = true
				}
				if len(got) != len(ids) {
					return fmt.Errorf("give details for all %d cards, got %d", len(ids), len(got))
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		byID := map[string]map[string]any{}
		for _, c := range out.Cards {
			byID[c.ID] = c.Fields
		}
		for i := range batch {
			c := &batch[i]
			for _, f := range fields {
				s, _ := byID[c.ID][f].(string)
				if v, ok := checkDetail(f, strings.TrimSpace(s), *c); ok {
					c.Set(f, v, engine.TrustExtracted)
				}
			}
		}
	}
	return in, nil
})

// checkDetail cleans a pulled-out value, or drops it (false) when it's
// empty, malformed or, for contact details, not in the card.
func checkDetail(field, v string, c engine.Card) (string, bool) {
	if v == "" || strings.EqualFold(v, "null") || strings.EqualFold(v, "unknown") {
		return "", false
	}
	v = cut(v, maxDetailChars)
	text := strings.ToLower(cardText(c))
	switch field {
	case "email":
		v = strings.ToLower(strings.TrimPrefix(v, "mailto:"))
		return v, emailRe.MatchString(v) && strings.Contains(text, v)
	case "phone":
		d := digitsOf(v)
		return v, len(d) >= 6 && len(d) <= 15 && strings.Contains(digitsOf(text), d)
	case "website":
		u := v
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		p, err := url.Parse(u)
		if err != nil || !strings.Contains(p.Hostname(), ".") {
			return "", false
		}
		host := strings.TrimPrefix(strings.ToLower(p.Hostname()), "www.")
		return u, strings.Contains(text, host)
	case "date":
		_, err := time.Parse("2006-01-02", v)
		return v, dateRe.MatchString(v) && err == nil
	case "time":
		return v, timeRe.MatchString(v)
	}
	return v, true
}

// cardText is everything the card says, for checking pulled-out details.
func cardText(c engine.Card) string {
	var b strings.Builder
	for k := range c.Fields {
		b.WriteString(c.Text(k))
		b.WriteByte('\n')
	}
	b.WriteString(c.Source)
	return b.String()
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

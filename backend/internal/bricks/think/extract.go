package think

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
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
	// A receipt or invoice.
	"vendor":         "the shop or company the receipt or invoice is from",
	"total":          "the total amount to pay, tax included: digits with a dot and no currency, e.g. \"11.50\"",
	"net":            "the total without tax: digits with a dot and no currency, e.g. \"9.66\"",
	"vat_amount":     "the tax (VAT) amount: digits with a dot and no currency, e.g. \"1.84\"",
	"vat_rate":       "the tax (VAT) rate in percent, digits only, e.g. \"19\"; several rates as \"7, 19\"",
	"currency":       "the currency as a three-letter code, e.g. \"EUR\"",
	"invoice_number": "the receipt or invoice number as printed",
	// A provider's report on a domain's email (DMARC). Laamo counts the
	// numbers itself and writes them in a summary above the file (see
	// mail/dmarc.go): they are copied from there, never worked out.
	"report_from":    "who sent the report, after \"Reporter:\" in the report summary",
	"report_domain":  "the domain the report is about, after \"Domain:\" in the report summary",
	"report_period":  "the days the report covers, after \"Period:\" in the report summary",
	"emails_total":   "the number after \"Emails:\" in the report summary, digits only",
	"emails_passed":  "the number after \"Passed DMARC:\" in the report summary, digits only",
	"emails_failed":  "the number after \"Failed DMARC:\" in the report summary, digits only",
	"failed_sources": "what stands after \"Failed sources:\" in the report summary, as written; \"none\" if it says none",
	"verdict":        "\"OK\" if no email failed; otherwise one sentence in plain words: how many failed and the reason the report summary gives for each failed source (a forwarding mailbox, or possibly someone else sending in the domain's name); never claim more than the summary says",
}

// amountFields are money amounts: kept only when their digits are in the
// card, so the AI can't add up, round or invent a sum for the books.
var amountFields = map[string]bool{"total": true, "net": true, "vat_amount": true}

// maxDetailChars caps one pulled-out value.
const maxDetailChars = 300

var (
	emailRe  = regexp.MustCompile(`^[^@\s<>"]+@[^@\s<>"]+\.[a-z]{2,}$`)
	dateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	amountRe = regexp.MustCompile(`^\d{1,9}\.\d{1,3}$`)
	timeRe   = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
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

	for _, real := range batches(in, 10) {
		batch, back := shortIDs(real)
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
		// Details are best effort: entries for unknown or repeated cards are
		// left out, and a card the AI skipped just gets none. Only an answer
		// that fits no card at all is sent back.
		err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: cardsJSON(batch)}}}, &out,
			func() error {
				for _, c := range out.Cards {
					if back[c.ID] != "" {
						return nil
					}
				}
				return fmt.Errorf("give an entry for each card id (%d cards)", len(batch))
			})
		if err != nil {
			return nil, err
		}
		byID := map[string]map[string]any{}
		for _, c := range out.Cards {
			if id := back[c.ID]; id != "" && byID[id] == nil {
				byID[id] = c.Fields
			}
		}
		for i := range real {
			c := &real[i]
			for _, f := range fields {
				s, _ := byID[c.ID][f].(string)
				if v, ok := checkDetail(f, strings.TrimSpace(s), *c); ok {
					c.Set(f, v, engine.TrustExtracted)
				}
			}
		}
	}
	// Only found: a card missing any of the details stops here (e.g. no
	// email, so nothing to write to).
	if cfg.Bool("only_found") {
		out := in[:0:0]
		for _, c := range in {
			if !slices.ContainsFunc(fields, func(f string) bool { return strings.TrimSpace(c.Text(f)) == "" }) {
				out = append(out, c)
			}
		}
		return out, nil
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
	if amountFields[field] {
		return checkAmount(v, text)
	}
	return v, true
}

// checkAmount tidies a money amount to digits with a dot ("1.234,50 €" →
// "1234.50") and keeps it only if the card shows it: as written, or without
// the zeros a receipt may leave out ("12,-" for 12.00).
func checkAmount(v, text string) (string, bool) {
	v = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			return r
		}
		return -1
	}, v)
	// The last dot or comma followed by one or two digits is the decimal
	// mark; the others group thousands.
	whole, cents := v, ""
	if i := strings.LastIndexAny(v, ".,"); i >= 0 && len(v)-i-1 >= 1 && len(v)-i-1 <= 2 {
		whole, cents = v[:i], v[i+1:]
	}
	whole = strings.TrimLeft(digitsOf(whole), "0")
	if whole == "" {
		whole = "0"
	}
	if !amountRe.MatchString(whole + "." + cents + "0") {
		return "", false
	}
	if len(cents) == 1 {
		cents += "0"
	}
	out := whole
	if cents != "" {
		out += "." + cents
	}
	digits := digitsOf(text)
	for _, d := range []string{whole + cents, whole + strings.TrimRight(cents, "0")} {
		if d != "" && d != "0" && strings.Contains(digits, d) {
			return out, true
		}
	}
	return "", false
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

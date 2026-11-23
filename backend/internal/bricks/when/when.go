// Package when contains the 🟡 When bricks. They start a run: the schedule
// itself lives on the helper's trigger (internal/schedule), the Tap Go answer
// arrives with the run, so at run time these bricks only check their input
// and hand an empty pile to the next brick.
package when

import (
	"context"
	"fmt"
	"time"

	"github.com/STGime/laamo/backend/internal/engine"
)

// Every… (when.every): runs on a schedule. The scheduler starts the run; the
// brick has nothing to add.
var Every = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
	return nil, nil
})

// Tap Go (when.tap): runs when the user taps Go. With a question, the answer
// comes with the run (trigger_data.answer) and is {answer} for later bricks.
var Tap = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	q := cfg.String("question")
	if q == "" {
		return nil, nil
	}
	if a, ok := rc.Value("answer"); !ok || a == "" {
		return nil, fmt.Errorf("the run started without an answer to the Tap Go question “%s”: %w", q, engine.ErrMissingValue)
	}
	return nil, nil
})

// Email this helper (when.email): runs when an email arrives at the helper's
// address (POST /api/inbound/email starts the run with the email). It hands
// on one email card. Everything in it is untrusted: anyone can write an
// email, so it can change what the helper writes, never who it sends to.
var Email = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, _ engine.Config) ([]engine.Card, error) {
	from, _ := rc.Value("trigger.from")
	if from == "" {
		// Started with the camera button instead of an email: the photos
		// are the run's input (Read the photo), and there's no email card.
		if photos, _ := rc.Value("trigger.photos"); photos != "" {
			return nil, nil
		}
		if rc.TestMode {
			return []engine.Card{sampleEmail(rc)}, nil
		}
		return nil, fmt.Errorf("the run started without an email: %w", engine.ErrMissingValue)
	}
	c := engine.Card{Kind: "email"}
	for _, f := range emailFields {
		v, _ := rc.Value("trigger." + f)
		c.Set(f, v, engine.TrustUntrusted)
	}
	// from_signed is Laamo's own DKIM check ("yes" or "no"), not something
	// the sender wrote, so it keeps the user's trust.
	signed, _ := rc.Value("trigger.from_signed")
	c.Set("from_signed", signed, engine.TrustUser)
	return []engine.Card{c}, nil
})

// emailFields: what an email card has (the catalog's adds for when.email),
// apart from from_signed.
var emailFields = []string{"from", "from_name", "subject", "text", "date", "attachments", "mail"}

// sampleEmail stands in for an email in a test run (Test in the builder):
// the bricks after it get something to work on.
func sampleEmail(rc *engine.RunContext) engine.Card {
	c := engine.Card{Kind: "email"}
	for _, f := range emailFields {
		c.Set(f, "", engine.TrustUntrusted)
	}
	c.Set("from_signed", "no", engine.TrustUser)
	c.Set("date", time.Now().UTC().Format(time.RFC3339), engine.TrustUntrusted)
	de := rc.Workspace.Locale == "de"
	pick := func(en, deText string) string {
		if de {
			return deText
		}
		return en
	}
	c.Set("from", "anna@example.com", engine.TrustUntrusted)
	c.Set("from_name", "Anna Schmidt", engine.TrustUntrusted)
	c.Set("subject", pick("Meeting next Thursday?", "Termin nächsten Donnerstag?"), engine.TrustUntrusted)
	c.Set("text", pick("Hi,\n\ncould we meet next Thursday at 10:00 to talk about the new offer? I'd bring the price list.\n\nBest,\nAnna",
		"Hallo,\n\nkönnen wir uns nächsten Donnerstag um 10 Uhr zum neuen Angebot treffen? Ich bringe die Preisliste mit.\n\nViele Grüße\nAnna"), engine.TrustUntrusted)
	return c
}

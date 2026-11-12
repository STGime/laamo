package think_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/think"
	"github.com/STGime/laamo/backend/internal/engine"
)

// businesses: two cards as Find businesses or Read a webpage would give them.
func businesses() []engine.Card {
	a := engine.Card{ID: "c0", Kind: "place"}
	a.Set("name", "Café Kuchenzeit", engine.TrustExtracted)
	a.Set("page_text", "Kontakt: hallo@kuchenzeit.de, Tel. 0341 / 123 45 67. Mo–Fr 8–18 Uhr.", engine.TrustUntrusted)
	a.Set("url", "https://www.kuchenzeit.de/kontakt", engine.TrustUntrusted)
	b := engine.Card{ID: "c1", Kind: "place"}
	b.Set("name", "Eiscafé Venezia", engine.TrustExtracted)
	b.Set("page_text", "Willkommen! Bestes Eis in Plagwitz.", engine.TrustUntrusted)
	return []engine.Card{a, b}
}

// Pull out details keeps what's in the card and drops what the model made up.
func TestExtract(t *testing.T) {
	m := &model{f: func(_ string, ids []string, _ int) string {
		return `{"cards":[
			{"id":"c0","fields":{"email":"Hallo@Kuchenzeit.de","phone":"0341 1234567","website":"kuchenzeit.de","opening_days":"Mo–Fr","date":"2026-13-01"}},
			{"id":"c1","fields":{"email":"info@venezia.de","phone":"0341 999999","website":null,"opening_days":null,"date":"2026-12-24"}}]}`
	}}
	out, err := brickstest.Contract(t, "think.extract", think.Extract, runCtx(t, "think.extract", m), businesses(),
		map[string]any{"fields": []any{"email", "phone", "website", "opening_days", "date"}})
	if err != nil {
		t.Fatal(err)
	}
	a, b := out[0], out[1]
	if a.Text("email") != "hallo@kuchenzeit.de" || a.Text("phone") != "0341 1234567" || a.Text("website") != "https://kuchenzeit.de" ||
		a.Text("opening_days") != "Mo–Fr" || a.Trust["email"] != engine.TrustExtracted {
		t.Fatalf("card 0: %v %v", a.Fields, a.Trust)
	}
	if a.Text("date") != "" {
		t.Fatalf("month 13 kept: %q", a.Text("date"))
	}
	// Not in the card: made up, so dropped. A real date stays.
	if b.Text("email") != "" || b.Text("phone") != "" || b.Text("date") != "2026-12-24" {
		t.Fatalf("card 1: %v", b.Fields)
	}
	if sys := m.reqs[0].System; !strings.Contains(sys, `"opening_days": free text: opening days`) || !strings.Contains(sys, "never guess") {
		t.Fatalf("prompt: %s", sys)
	}
}

func TestWriteEach(t *testing.T) {
	m := &model{f: func(system string, _ []string, attempt int) string {
		if attempt == 1 && strings.Contains(system, "German") {
			return `{"subject":"Kuchen für [Firma]","body":"Hallo!"}` // a placeholder: repaired
		}
		return `{"subject":"Frische Kuchen für Ihr Café","body":"Guten Tag,\n\nwir backen jeden Morgen frisch.\n\nViele Grüße"}`
	}}
	rc := runCtx(t, "think.write", m)
	rc.Workspace.Locale = "de"
	rc.Workspace.Profile = map[string]any{"business": "Bäckerei Klein", "offer": "Kuchen für Cafés", "signature": "Anna Klein\nBäckerei Klein"}
	out, err := brickstest.Contract(t, "think.write", think.Write, rc, businesses(),
		map[string]any{"what": "eine kurze E-Mail an {name} mit {my offer}", "tone": 0.8, "mode": "each", "signature": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Text("subject") != "Frische Kuchen für Ihr Café" ||
		!strings.HasSuffix(out[0].Text("body"), "Viele Grüße\n\nAnna Klein\nBäckerei Klein") || out[0].Trust["body"] != engine.TrustExtracted {
		t.Fatalf("drafts %v", out[0].Fields)
	}
	first := m.reqs[0]
	if !strings.Contains(first.Messages[0].Content, `"eine kurze E-Mail an Café Kuchenzeit mit Kuchen für Cafés"`) ||
		!strings.Contains(first.System, `"Sie"`) || !strings.Contains(first.System, "no name or signature") ||
		!strings.Contains(first.System, "Their business: Bäckerei Klein.") {
		t.Fatalf("prompt:\n%s\n%s", first.System, first.Messages[0].Content)
	}
	if len(m.reqs) != 4 { // each card's first try had a placeholder
		t.Fatalf("%d calls", len(m.reqs))
	}
}

// The reader's own email or phone from the card never ends up as "contact
// us at …" in the draft.
func TestWriteNotTheirContact(t *testing.T) {
	m := &model{f: func(_ string, _ []string, attempt int) string {
		if attempt == 1 {
			return `{"subject":"Kuchen","body":"Rufen Sie uns an: 0341 / 123 45 67 oder hallo@kuchenzeit.de."}`
		}
		return `{"subject":"Kuchen","body":"Rufen Sie uns an: 0341 555 0000."}`
	}}
	rc := runCtx(t, "think.write", m)
	rc.Workspace.Profile = map[string]any{"offer": "Kuchen, Tel. 0341 555 0000"}
	out, err := brickstest.Contract(t, "think.write", think.Write, rc, businesses()[:1],
		map[string]any{"what": "an email offering {my offer}", "tone": 0.5, "mode": "each", "signature": false})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.reqs) != 2 || !strings.Contains(m.reqs[1].Messages[2].Content, "the reader's contact detail") ||
		out[0].Text("body") != "Rufen Sie uns an: 0341 555 0000." {
		t.Fatalf("%d calls, body %q", len(m.reqs), out[0].Text("body"))
	}
}

func TestWriteAllCasual(t *testing.T) {
	m := &model{f: func(string, []string, int) string { return `{"subject":"","body":"Hey, here's what's new this week."}` }}
	rc := runCtx(t, "think.write", m)
	out, err := brickstest.Contract(t, "think.write", think.Write, rc, businesses(),
		map[string]any{"what": "a post about this week", "tone": 0.1, "mode": "all", "signature": true})
	if err != nil {
		t.Fatal(err)
	}
	// One draft for all; no signature in About me, so none added.
	if len(out) != 1 || out[0].Kind != "draft" || out[0].Text("body") != "Hey, here's what's new this week." || out[0].Text("subject") != "" {
		t.Fatalf("draft %+v", out)
	}
	if sys := m.reqs[0].System; !strings.Contains(sys, "casual") || !strings.Contains(sys, "sender's name") {
		t.Fatalf("prompt: %s", sys)
	}
	if n := len(m.reqs); n != 1 {
		t.Fatal(fmt.Sprint(n, " calls"))
	}
}

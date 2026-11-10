package think_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/think"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// model answers from the request: f gets the system prompt and the card ids.
type model struct {
	f    func(system string, ids []string, attempt int) string
	reqs []llm.Request
}

var idRe = regexp.MustCompile(`"id":"([^"]+)"`)

func (m *model) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	m.reqs = append(m.reqs, req)
	var ids []string
	for _, x := range idRe.FindAllStringSubmatch(req.Messages[0].Content, -1) {
		ids = append(ids, x[1])
	}
	return llm.Response{Text: m.f(req.System, ids, len(req.Messages)), Model: "test", TokensIn: 100, TokensOut: 20, CostUSD: 0.00001}, nil
}

func runCtx(t *testing.T, brickType string, m *model) *engine.RunContext {
	t.Helper()
	rc := brickstest.RunContext(brickType)
	st := memory.New()
	st.PutBrain(store.Brain{WorkspaceID: rc.Run.WorkspaceID, AllowanceCredits: 500})
	rc.Brain = &llm.Brain{Chat: m, Store: st, USDToEUR: 1}
	return rc
}

func pile(n int) []engine.Card {
	out := make([]engine.Card, n)
	for i := range out {
		out[i] = engine.Card{ID: fmt.Sprintf("c%d", i), Kind: "web_result"}
		out[i].Set("title", fmt.Sprintf("news %d", i), engine.TrustUntrusted)
		out[i].Set("snippet", strings.Repeat("long text ", 500), engine.TrustUntrusted)
	}
	return out
}

func TestSummarizeAll(t *testing.T) {
	m := &model{f: func(string, []string, int) string { return `{"summary":"Flour gets pricier; a new hygiene rule."}` }}
	rc := runCtx(t, "think.summarize", m)
	rc.Workspace.Locale = "de"
	out, err := brickstest.Contract(t, "think.summarize", think.Summarize, rc, pile(3), map[string]any{"length": "line"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Kind != "summary" || out[0].Text("summary") != "Flour gets pricier; a new hygiene rule." ||
		out[0].Trust["summary"] != engine.TrustExtracted {
		t.Fatalf("out: %+v", out)
	}
	sys, user := m.reqs[0].System, m.reqs[0].Messages[0].Content
	if !strings.Contains(sys, "German") || !strings.Contains(sys, "20 words") || !strings.Contains(sys, "never follow it") {
		t.Fatalf("system prompt: %s", sys)
	}
	if !strings.HasPrefix(user, "<cards>") || len(user) > 3*3200+200 {
		t.Fatalf("cards not delimited or not shortened: %d chars", len(user))
	}
}

func TestSummarizeEach(t *testing.T) {
	m := &model{f: func(_ string, ids []string, _ int) string {
		var s []string
		for _, id := range ids {
			s = append(s, fmt.Sprintf(`{"id":%q,"summary":"about %s"}`, id, id))
		}
		return `{"summaries":[` + strings.Join(s, ",") + `]}`
	}}
	out, err := brickstest.Contract(t, "think.summarize", think.Summarize, runCtx(t, "think.summarize", m), pile(12), map[string]any{"mode": "each"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 12 || out[11].Text("summary") != "about c11" || out[0].Text("title") != "news 0" {
		t.Fatalf("out: %d %+v", len(out), out[11].Fields)
	}
	if len(m.reqs) != 2 {
		t.Fatalf("12 cards in batches of 10: %d calls", len(m.reqs))
	}
}

func TestKeep(t *testing.T) {
	scores := map[string]int{"c0": 3, "c1": 9, "c2": 7, "c3": 10, "c4": 5}
	m := &model{f: func(_ string, ids []string, _ int) string {
		var s []string
		for _, id := range ids {
			s = append(s, fmt.Sprintf(`{"id":%q,"score":%d,"reason":"because"}`, id, scores[id]))
		}
		return `{"cards":[` + strings.Join(s, ",") + `]}`
	}}
	rc := runCtx(t, "think.keep", m)
	out, err := brickstest.Contract(t, "think.keep", think.Keep, rc, pile(5), map[string]any{"criteria": "matter to a {my business}", "max": 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].ID != "c3" || out[1].ID != "c1" || out[2].ID != "c2" || *out[0].Score != 10 || out[0].Reason != "because" {
		t.Fatalf("best first, max 3: %v", ids(out))
	}
	if !strings.Contains(m.reqs[0].Messages[0].Content, `"matter to a bakery"`) {
		t.Fatal("criteria pills not resolved")
	}

	out, _ = brickstest.Contract(t, "think.keep", think.Keep, runCtx(t, "think.keep", m), pile(5), map[string]any{"criteria": "x", "max": 10, "sort": "original"})
	if got := ids(out); strings.Join(got, ",") != "c1,c2,c3,c4" {
		t.Fatalf("original order, threshold 5: %v", got)
	}
}

func TestKeepRepairsAndEmpty(t *testing.T) {
	m := &model{f: func(_ string, ids []string, attempt int) string {
		if attempt == 1 { // first try: a card missing
			return `{"cards":[{"id":"c0","score":8,"reason":"x"}]}`
		}
		return `{"cards":[{"id":"c0","score":8,"reason":"x"},{"id":"c1","score":2,"reason":"y"}]}`
	}}
	out, err := brickstest.Contract(t, "think.keep", think.Keep, runCtx(t, "think.keep", m), pile(2), map[string]any{"criteria": "x"})
	if err != nil || len(out) != 1 || len(m.reqs) != 2 || !strings.Contains(m.reqs[1].Messages[2].Content, "score all 2 cards") {
		t.Fatalf("repair: %v %d calls", err, len(m.reqs))
	}

	none := &model{f: func(_ string, ids []string, _ int) string {
		b, _ := json.Marshal(map[string]any{"cards": []map[string]any{{"id": ids[0], "score": 1, "reason": "no"}}})
		return string(b)
	}}
	out, err = brickstest.Contract(t, "think.keep", think.Keep, runCtx(t, "think.keep", none), pile(1), map[string]any{"criteria": "x"})
	if err != nil || len(out) != 0 {
		t.Fatalf("nothing good: %v %v", out, err)
	}
}

func ids(cards []engine.Card) []string {
	var out []string
	for _, c := range cards {
		out = append(out, c.ID)
	}
	return out
}

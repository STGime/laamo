// Command runstack runs a stack (or a Shelf template) locally and prints the
// pile after every brick, with cost. Everything stays in memory: nothing is
// written to Eurobase.
//
//	runstack [flags] stack-or-template.json
//
//	-live            real Brave search, real AI (LLM key), real chat webhook
//	                 (default: offline fixtures: fake search and fake AI)
//	-webhook URL     Discord or Slack webhook for Post to chat (default: print)
//	-profile k=v,…   About me facts (default business=bakery,city=Leipzig)
//	-lang en|de      workspace language
//	-tz Zone         workspace timezone (default Europe/Berlin)
//	-set b2.query=…  set a brick setting (repeatable), e.g. a template's
//	                 setup answers
//	-answer text     the Tap Go answer ({answer})
//	-test            test mode (3 cards, Do bricks to you)
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type setFlags []string

func (s *setFlags) String() string     { return strings.Join(*s, ",") }
func (s *setFlags) Set(v string) error { *s = append(*s, v); return nil }

type options struct {
	live    bool
	webhook string
	profile string
	lang    string
	tz      string
	answer  string
	test    bool
	sets    setFlags
}

func main() {
	var o options
	flag.BoolVar(&o.live, "live", false, "use real search, AI and chat (default: offline fixtures)")
	flag.StringVar(&o.webhook, "webhook", "", "Discord or Slack webhook for Post to chat (default: print the message)")
	flag.StringVar(&o.profile, "profile", "business=bakery,city=Leipzig", "About me facts, k=v,…")
	flag.StringVar(&o.lang, "lang", "en", "workspace language (en, de)")
	flag.StringVar(&o.tz, "tz", "Europe/Berlin", "workspace timezone")
	flag.StringVar(&o.answer, "answer", "", "the Tap Go answer ({answer})")
	flag.BoolVar(&o.test, "test", false, "test mode")
	flag.Var(&o.sets, "set", "brick setting, e.g. b2.query=news (repeatable)")
	flag.Usage = func() {
		_, _ = fmt.Fprintln(os.Stderr, "usage: runstack [flags] stack-or-template.json")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), os.Stdout, flag.Arg(0), o); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "runstack:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, w io.Writer, path string, o options) error {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: a dev tool reading the file it was given
	if err != nil {
		return err
	}
	stack, chatFills, err := loadStack(raw)
	if err != nil {
		return err
	}
	if err := applySets(&stack, o.sets); err != nil {
		return err
	}

	cat := catalog.Default()
	st := memory.New()
	profile := map[string]any{}
	for _, kv := range strings.Split(o.profile, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.TrimSpace(k) != "" {
			profile[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	user := store.NewID()
	ws := st.PutWorkspace(store.Workspace{UserID: user, Name: "runstack", Timezone: o.tz, Locale: o.lang, Profile: profile})
	st.PutBrain(store.Brain{WorkspaceID: ws.ID, UserID: user, AllowanceCredits: 500})

	// A chat place for Post to chat, encrypted like in production.
	keyBytes := make([]byte, 32)
	_, _ = rand.Read(keyBytes)
	keys, _ := crypto.ParseKeyring("dev:"+base64.StdEncoding.EncodeToString(keyBytes), "")
	place := store.Place{ID: store.NewID(), WorkspaceID: ws.ID, UserID: user, Type: "discord", Label: "runstack"}
	webhook := "https://discord.com/api/webhooks/0/print"
	if o.webhook != "" {
		webhook = o.webhook
		if strings.Contains(webhook, "hooks.slack.com") {
			place.Type = "slack"
		}
		if err := chat.CheckWebhook(place.Type, webhook); err != nil {
			return err
		}
	}
	sealed, err := keys.Encrypt([]byte(webhook), crypto.AAD("places", place.ID, "secret"))
	if err != nil {
		return err
	}
	place.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := st.CreatePlace(ctx, &place); err != nil {
		return err
	}
	for i := range stack.Bricks {
		sb := &stack.Bricks[i]
		if sb.Type == "do.chat" && (chatFills[sb.ID] || sb.Config["place"] == nil || sb.Config["place"] == "") {
			if sb.Config == nil {
				sb.Config = map[string]any{}
			}
			sb.Config["place"] = place.ID
		}
	}

	if errs := catalog.Errors(cat.Validate(stack, catalog.Options{})); len(errs) > 0 {
		for _, e := range errs {
			_, _ = fmt.Fprintf(w, "✗ %s %s: %s\n", e.Brick, e.Code, e.Message)
		}
		return errors.New("the stack isn't valid")
	}

	deps, brain, err := services(o, w, st, keys)
	if err != nil {
		return err
	}
	stackJSON, _ := json.Marshal(stack)
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: user, Name: helperName(raw, path), Status: "active"})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: user, HelperID: h.ID, Version: 1, Stack: stackJSON})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)

	eng := &engine.Engine{Store: st, Catalog: cat, Bricks: bricks.All(deps), Brain: brain, Blobs: &engine.MemoryBlobs{},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	data := map[string]any{}
	if o.answer != "" {
		data["answer"] = o.answer
	}
	trigger := "manual"
	if o.test {
		trigger = "test"
	}
	r, err := eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: trigger, TriggerData: data})
	if err != nil {
		return err
	}
	mode := "offline fixtures"
	if o.live {
		mode = "live"
	}
	_, _ = fmt.Fprintf(w, "▶ %s (%s, %s)\n\n", h.Name, mode, o.lang)
	start := time.Now()
	if err := eng.Execute(ctx, r.ID); err != nil {
		_, _ = fmt.Fprintf(w, "! the run would be retried: %v\n", err)
	}
	return report(ctx, w, st, cat, stack, r.ID, o.lang, time.Since(start), place)
}

// loadStack reads a stack, or a template (its stack plus which Post to chat
// settings the setup questions fill).
func loadStack(raw []byte) (catalog.Stack, map[string]bool, error) {
	var probe struct {
		Stack *catalog.Stack `json:"stack"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return catalog.Stack{}, nil, err
	}
	fills := map[string]bool{}
	if probe.Stack != nil {
		var t catalog.Template
		if err := json.Unmarshal(raw, &t); err != nil {
			return catalog.Stack{}, nil, err
		}
		for _, q := range t.SetupQuestions {
			for _, f := range q.Fills {
				if f.Setting == "place" {
					fills[f.Brick] = true
				}
			}
		}
		return t.Stack, fills, nil
	}
	var s catalog.Stack
	err := json.Unmarshal(raw, &s)
	return s, fills, err
}

func helperName(raw []byte, path string) string {
	var t struct {
		Name catalog.Text `json:"name"`
	}
	if json.Unmarshal(raw, &t) == nil && t.Name.En != "" {
		return t.Name.En
	}
	return strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
}

func applySets(st *catalog.Stack, sets []string) error {
	for _, s := range sets {
		key, val, ok := strings.Cut(s, "=")
		id, setting, ok2 := strings.Cut(key, ".")
		if !ok || !ok2 {
			return fmt.Errorf("-set %q: want brick.setting=value", s)
		}
		i := slices.IndexFunc(st.Bricks, func(b catalog.StackBrick) bool { return b.ID == id })
		if i < 0 {
			return fmt.Errorf("-set %q: no brick %s", s, id)
		}
		if st.Bricks[i].Config == nil {
			st.Bricks[i].Config = map[string]any{}
		}
		var v any = val
		var n float64
		if err := json.Unmarshal([]byte(val), &n); err == nil {
			v = n
		}
		st.Bricks[i].Config[setting] = v
	}
	return nil
}

// services returns real or fixture services.
func services(o options, w io.Writer, st *memory.Store, keys *crypto.Keyring) (bricks.Deps, *llm.Brain, error) {
	deps := bricks.Deps{Store: st, Keys: keys, Chat: printSender{w}, SearchCostEUR: 0.005}
	if !o.live {
		deps.Search = fixtureSearch{}
		return deps, &llm.Brain{Chat: fixtureBrain{}, Model: "fixture", Store: st}, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return deps, nil, err
	}
	if cfg.BraveSearchAPIKey == "" || cfg.LLMAPIKey == "" {
		return deps, nil, errors.New("-live needs BRAVE_SEARCH_API_KEY and an LLM key (FAL_KEY) in the environment (task runstack loads .env)")
	}
	var p search.Provider = &search.Brave{APIKey: cfg.BraveSearchAPIKey, Limit: search.NewLimiter(cfg.BraveRPS)}
	if cfg.SearchCacheDir != "" {
		p = &search.Cached{Provider: p, Dir: cfg.SearchCacheDir, TTL: 24 * time.Hour}
	}
	deps.Search, deps.SearchCostEUR = p, cfg.SearchCostEUR
	if o.webhook != "" {
		deps.Chat = &chat.Sender{}
	}
	brain := &llm.Brain{Chat: &llm.Client{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Auth: cfg.LLMAuth, Model: cfg.LLMModel},
		Model: cfg.LLMModel, Store: st}
	return deps, brain, nil
}

// report prints every step's pile and the totals.
func report(ctx context.Context, w io.Writer, st *memory.Store, cat *catalog.Catalog, stack catalog.Stack, runID, lang string,
	took time.Duration, place store.Place) error {
	r, err := st.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	steps, err := st.ListSteps(ctx, runID)
	if err != nil {
		return err
	}
	for _, s := range steps {
		i := slices.IndexFunc(stack.Bricks, func(b catalog.StackBrick) bool { return b.ID == s.BrickID })
		sentence := s.BrickType
		if i >= 0 {
			sentence = strings.ReplaceAll(cat.Sentence(stack.Bricks[i], lang), place.ID, "#"+place.Label)
		}
		mark := "✓"
		if s.Status != "done" {
			mark = "✗"
		}
		_, _ = fmt.Fprintf(w, "%s %s  %s  (%d → %d cards, €%.4f)\n", mark, s.BrickID, sentence, s.CardsIn, s.CardsOut, float64(s.CostEUR))
		for _, l := range s.Log {
			_, _ = fmt.Fprintf(w, "    ! %v\n", l["msg"])
		}
		cards, _ := st.ListCards(ctx, s.ID)
		for j, c := range cards {
			if j == 5 {
				_, _ = fmt.Fprintf(w, "    … %d more\n", len(cards)-5)
				break
			}
			_, _ = fmt.Fprintf(w, "    • %s\n", cardLine(c))
		}
	}
	_, _ = fmt.Fprintf(w, "\n%s · €%.4f · %d+%d AI tokens · %s\n", strings.ToUpper(r.Status), float64(r.CostEUR), r.TokensIn, r.TokensOut, took.Round(time.Millisecond))
	for _, f := range st.FeedCards() {
		if f.RunID != nil && *f.RunID == runID {
			_, _ = fmt.Fprintf(w, "Feed: %s", f.Title)
			if f.Body != nil {
				_, _ = fmt.Fprintf(w, "\n  %s", strings.ReplaceAll(*f.Body, "\n", "\n  "))
			}
			_, _ = fmt.Fprintln(w)
		}
	}
	if r.Status != store.RunDone {
		return fmt.Errorf("run %s", r.Status)
	}
	return nil
}

func cardLine(c store.Card) string {
	text := func(k string) string { s, _ := c.Data[k].(string); return s }
	line := text("title")
	for _, k := range []string{"name", "summary", "body", "text"} {
		if line == "" {
			line = text(k)
		}
	}
	if c.Score != nil {
		line = fmt.Sprintf("[%v] %s", float64(*c.Score), line)
	}
	if c.Reason != nil && *c.Reason != "" {
		line += " — " + *c.Reason
	}
	r := []rune(strings.ReplaceAll(line, "\n", " "))
	if len(r) > 160 {
		return string(r[:159]) + "…"
	}
	return string(r)
}

// printSender shows chat messages instead of posting them.
type printSender struct{ w io.Writer }

func (p printSender) Send(_ context.Context, placeType, _ string, m chat.Message) error {
	_, _ = fmt.Fprintf(p.w, "    ┌ %s message (printed, not sent)\n    │ %s\n    └\n", placeType, strings.ReplaceAll(m.Text, "\n", "\n    │ "))
	return nil
}

// fixtureSearch returns made-up results (offline mode).
type fixtureSearch struct{}

func (fixtureSearch) Search(_ context.Context, q search.Query) ([]search.Result, error) {
	topics := []string{"Flour prices rise 8% in December", "New hygiene rules for food businesses from 2027",
		"Football: local team wins 3-1", "Christmas market stall fees for bakers drop", "City opens a new tram line",
		"Energy prices for small businesses fall", "Bakery chain closes 12 branches", "Weather: first snow expected"}
	var out []search.Result
	for i := range min(q.Count, len(topics)) {
		out = append(out, search.Result{Title: topics[i], URL: fmt.Sprintf("https://example.com/news/%d", i),
			Snippet: "Fixture result for “" + q.Q + "”.", PublishedAt: time.Now().Add(-time.Duration(i) * time.Hour).UTC()})
	}
	return out, nil
}

// fixtureBrain answers Keep and Sum it up without a model (offline mode):
// it keeps cards whose title mentions business words and lists them.
type fixtureBrain struct{}

func (fixtureBrain) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	var cards []struct {
		ID     string            `json:"id"`
		Fields map[string]string `json:"fields"`
	}
	content := req.Messages[len(req.Messages)-1].Content
	if i, j := strings.Index(content, "<cards>"), strings.LastIndex(content, "</cards>"); i >= 0 && j > i {
		_ = json.Unmarshal([]byte(content[i+len("<cards>"):j]), &cards)
	}
	var text string
	switch {
	case strings.Contains(req.System, `"cards":[`): // Keep the good ones
		var out []map[string]any
		for _, c := range cards {
			score := 2
			t := strings.ToLower(c.Fields["title"])
			if strings.Contains(t, "flour") || strings.Contains(t, "bak") || strings.Contains(t, "food") || strings.Contains(t, "business") {
				score = 8
			}
			out = append(out, map[string]any{"id": c.ID, "score": score, "reason": "fixture judgement"})
		}
		b, _ := json.Marshal(map[string]any{"cards": out})
		text = string(b)
	case strings.Contains(req.System, `"summaries"`): // Sum it up, one per card
		var out []map[string]string
		for _, c := range cards {
			out = append(out, map[string]string{"id": c.ID, "summary": c.Fields["title"]})
		}
		b, _ := json.Marshal(map[string]any{"summaries": out})
		text = string(b)
	default: // Sum it up, one for all
		var titles []string
		for _, c := range cards {
			titles = append(titles, "• "+c.Fields["title"])
		}
		b, _ := json.Marshal(map[string]string{"summary": strings.Join(titles, "\n")})
		text = string(b)
	}
	return llm.Response{Text: text, Model: "fixture", TokensIn: 500, TokensOut: 100, CostUSD: 0.000001, FinishReason: "stop"}, nil
}

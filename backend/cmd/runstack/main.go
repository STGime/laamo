// Command runstack runs a stack (or a Shelf template) locally and prints the
// pile after every brick, with cost. By default everything stays in memory;
// with -workspace the run is written to that Eurobase workspace, so its
// Feed cards show up in the app (and Ask me cards wait for you there).
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
//	-decide how      answers Ask me cards: approve (default; picks all,
//	                 answers -reply), skip, or stop (leave the run waiting)
//	-reply text      the answer to Ask me something
//	-test            test mode (3 cards, Do bricks to you)
//	-workspace id    run in this Eurobase workspace (implies -live and
//	                 -decide stop; the worker resumes after you decide)
package main

import (
	"context"
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
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/osm"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/web"
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
	decide  string
	reply   string
	test    bool
	sets    setFlags
	// workspace: write to this real workspace instead of memory.
	workspace string
}

func main() {
	var o options
	flag.BoolVar(&o.live, "live", false, "use real search, AI and chat (default: offline fixtures)")
	flag.StringVar(&o.webhook, "webhook", "", "Discord or Slack webhook for Post to chat (default: print the message)")
	flag.StringVar(&o.profile, "profile", "", "About me facts, k=v,… (default: a bakery in Leipzig)")
	flag.StringVar(&o.lang, "lang", "en", "workspace language (en, de)")
	flag.StringVar(&o.tz, "tz", "Europe/Berlin", "workspace timezone")
	flag.StringVar(&o.answer, "answer", "", "the Tap Go answer ({answer})")
	flag.StringVar(&o.decide, "decide", "", "Ask me cards: approve, skip or stop (default: approve; stop with -workspace)")
	flag.StringVar(&o.workspace, "workspace", "", "run in this Eurobase workspace (cards show up in the app)")
	flag.StringVar(&o.reply, "reply", "", "the answer to Ask me something")
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
	o.lang = strings.TrimSpace(o.lang)
	if o.lang != "en" && o.lang != "de" {
		return fmt.Errorf("-lang %q: use en or de", o.lang)
	}
	if o.workspace != "" {
		o.live = true
		if o.decide == "" {
			o.decide = "stop"
		}
	}
	if o.decide == "" {
		o.decide = "approve"
	}
	if o.profile == "" { // the default About me facts, in the workspace language
		o.profile = map[string]string{"en": "business=bakery,city=Leipzig", "de": "business=Bäckerei,city=Leipzig"}[o.lang]
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: a dev tool reading the file it was given
	if err != nil {
		return err
	}
	env, err := setup(ctx, o, raw, path)
	if err != nil {
		return err
	}
	if o.workspace != "" {
		o.lang = env.ws.Locale // the workspace's language, About me and brain apply
	}
	stack, chatFills, err := loadStack(raw, o.lang)
	if err != nil {
		return err
	}
	if err := applySets(&stack, o.sets); err != nil {
		return err
	}

	cat := catalog.Default()
	st, ws, user, keys := env.st, env.ws, env.ws.UserID, env.keys
	needsChat := slices.ContainsFunc(stack.Bricks, func(b catalog.StackBrick) bool { return b.Type == "do.chat" })
	if o.workspace != "" && needsChat && o.webhook == "" {
		return errors.New("this stack posts to chat: with -workspace, pass -webhook (a real Discord or Slack webhook)")
	}
	if needsChat && keys == nil {
		return errors.New("this stack posts to chat: set LAAMO_MASTER_KEYS (see docs/security/secrets.md)")
	}

	// A chat place for Post to chat, encrypted like in production.
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
	if needsChat {
		sealed, err := keys.Encrypt([]byte(webhook), crypto.AAD("places", place.ID, "secret"))
		if err != nil {
			return err
		}
		place.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
		if err := st.CreatePlace(ctx, &place); err != nil {
			return err
		}
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
	h, err := env.helper(ctx, helperName(raw, path), helperAvatar(raw), stackJSON)
	if err != nil {
		return err
	}

	eng := &engine.Engine{Store: st, Catalog: cat, Bricks: bricks.All(deps), Brain: brain, Blobs: env.blobs, Resolve: tmpl.Config,
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
	if o.workspace != "" {
		mode = "live, in workspace " + ws.Name
	}
	_, _ = fmt.Fprintf(w, "▶ %s (%s, %s)\n\n", h.Name, mode, o.lang)
	start := time.Now()
	if err := eng.Execute(ctx, r.ID); err != nil {
		_, _ = fmt.Fprintf(w, "! the run would be retried: %v\n", err)
	}
	// Ask me bricks: answer the Feed card as -decide says and go on.
	for range 10 {
		cur, err := st.GetRun(ctx, r.ID)
		if err != nil || cur.Status != store.RunWaiting || o.decide == "stop" {
			break
		}
		if err := decide(ctx, w, st, cur, o); err != nil {
			return err
		}
		if err := eng.Execute(ctx, r.ID); err != nil {
			_, _ = fmt.Fprintf(w, "! the run would be retried: %v\n", err)
		}
	}
	if err := report(ctx, w, st, cat, stack, r.ID, o.lang, time.Since(start), place); err != nil {
		if cur, _ := st.GetRun(ctx, r.ID); o.workspace != "" && cur.Status == store.RunWaiting {
			_, _ = fmt.Fprintln(w, "\n⏸ Waiting for you: answer the card in the app's Feed. With `task dev` running, the worker goes on.")
			return nil
		}
		return err
	}
	return nil
}

func helperOf(ctx context.Context, st store.Store, f store.FeedCard) string {
	if f.HelperID == nil {
		return ""
	}
	h, _ := st.GetHelper(ctx, *f.HelperID)
	return h.Name
}

// decide answers the Feed card a waiting run waits on, like the app would.
func decide(ctx context.Context, w io.Writer, st store.Store, r store.Run, o options) error {
	fc, err := st.GetFeedCard(ctx, *r.WaitingOnCardID)
	if err != nil {
		return err
	}
	shown, err := st.ListCards(ctx, fc.ID)
	if err != nil {
		return err
	}
	var all []string
	for _, c := range shown {
		all = append(all, c.CardID)
	}
	state, d := engine.StateApproved, engine.Decision{}
	switch {
	case fc.Type == engine.FeedQuestion:
		state, d.Answer = engine.StateAnswered, o.reply
		if d.Answer == "" {
			return fmt.Errorf("%q needs an answer: use -reply", deref(fc.Body))
		}
	case fc.Type == engine.FeedPick && o.decide == "skip":
		state = engine.StateAnswered
	case fc.Type == engine.FeedPick:
		state, d.Picked = engine.StateAnswered, all
	case o.decide == "skip":
		state, d.Skip = engine.StateSkipped, all
	}
	if err := engine.CheckDecision(fc, shown, state, d); err != nil {
		return err
	}
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, err := st.UpdateFeedCard(ctx, fc.ID, store.FeedCardPatch{State: &state, Decision: m}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "    ⏸ %s: %s → %s (-decide %s)\n", fc.Type, deref(fc.Body), state, o.decide)
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// loadStack reads a stack, or a template (its stack plus which Post to chat
// settings the setup questions fill), in the workspace language.
func loadStack(raw []byte, lang string) (catalog.Stack, map[string]bool, error) {
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
		return t.StackFor(lang), fills, nil
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
func services(o options, w io.Writer, st store.Store, keys *crypto.Keyring) (bricks.Deps, *llm.Brain, error) {
	deps := bricks.Deps{Store: st, Keys: keys, Chat: printSender{w}, SearchCostEUR: 0.005}
	if !o.live {
		deps.Search, deps.Pages = fixtureSearch{}, fixturePages{}
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
	deps.Pages = &web.Fetcher{}
	deps.Businesses = &osm.Client{NominatimURL: cfg.OSMNominatimURL, OverpassURL: cfg.OSMOverpassURL}
	if o.webhook != "" {
		deps.Chat = &chat.Sender{}
	}
	brain := &llm.Brain{Chat: &llm.Client{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Auth: cfg.LLMAuth, Model: cfg.LLMModel},
		Model: cfg.LLMModel, Store: st}
	return deps, brain, nil
}

// report prints every step's pile and the totals.
func report(ctx context.Context, w io.Writer, st store.Store, cat *catalog.Catalog, stack catalog.Stack, runID, lang string,
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
		var adds []string
		if i >= 0 {
			sentence = strings.ReplaceAll(cat.Sentence(stack.Bricks[i], lang), place.ID, "#"+place.Label)
			if b, ok := cat.Brick(stack.Bricks[i].Type); ok {
				adds, _ = b.AddsFor(b.Config(stack.Bricks[i].Config))
			}
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
			_, _ = fmt.Fprintf(w, "    • %s\n", cardLine(c, adds))
		}
	}
	_, _ = fmt.Fprintf(w, "\n%s · €%.4f · %d+%d AI tokens · %s\n", strings.ToUpper(r.Status), float64(r.CostEUR), r.TokensIn, r.TokensOut, took.Round(time.Millisecond))
	feed, _ := st.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: r.WorkspaceID, Limit: 50})
	for _, f := range slices.Backward(feed) {
		if f.RunID != nil && *f.RunID == runID {
			_, _ = fmt.Fprintf(w, "Feed: %s · %s", helperOf(ctx, st, f), f.Title)
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

// cardLine shows a card's title, score and reason, and what the step
// added to it (a page's text, a summary).
func cardLine(c store.Card, adds []string) string {
	text := func(k string) string {
		switch v := c.Data[k].(type) {
		case string:
			return v
		case map[string]any: // stored in project storage
			return fmt.Sprintf("(%v bytes in storage)", v["bytes"])
		}
		return ""
	}
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
	line = strings.ReplaceAll(line, "\n", " ")
	for _, k := range adds {
		if v := text(k); v != "" && v != line && k != "title" {
			v = strings.Join(strings.Fields(v), " ")
			if r := []rune(v); len(r) > 100 {
				v = fmt.Sprintf("%s… (%d chars)", string(r[:100]), len(r))
			}
			line += "\n      " + k + ": " + v
		}
	}
	return clipLine(line)
}

func clipLine(line string) string {
	var out []string
	for l := range strings.SplitSeq(line, "\n") {
		if r := []rune(l); len(r) > 160 {
			l = string(r[:159]) + "…"
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
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

// fixturePages returns a made-up page for any URL (offline mode).
type fixturePages struct{}

func (fixturePages) Read(_ context.Context, u string) (web.Page, error) {
	return web.Page{URL: u, Title: "Fixture page", Text: "Made-up text of " + u + ". Flour prices rise in December.",
		Emails: []string{"info@example.com"}}, nil
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

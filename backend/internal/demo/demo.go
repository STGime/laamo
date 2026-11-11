// Package demo runs a sample helper for visitors without an account (#75):
// one allow-listed template, in memory, in Test mode, without its Ask and Do
// bricks. Nothing is saved and nothing is sent; the visitor sees the result
// card and can keep the helper after signing up.
//
// The endpoint is public, so it must not become a free search or AI proxy:
// every run needs a fresh proof of work, inputs are short plain words that
// only fill the template's own query, each IP gets a few runs, all demos
// share a small daily budget, and common inputs come from a cache.
package demo

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/bits"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/shelf"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// Templates a visitor may try: they only look and think.
var Templates = []string{"morning-briefing", "weekend-scout"}

var (
	ErrInput  = errors.New("bad demo input")
	ErrProof  = errors.New("bad or used proof of work")
	ErrLimit  = errors.New("too many demo runs, try again later")
	ErrBudget = errors.New("the demo is resting for today")
)

// Limits for the public demo.
type Limits struct {
	Bits      int           // proof of work: leading zero bits of sha256(token:nonce)
	PerIP     int           // runs per IP (IPv6: per /64) …
	PerIPDay  int           // … and per day
	Window    time.Duration // … within this window
	BudgetEUR float64       // all demos per day
	RunEUR    float64       // one run's cost limit
	Parallel  int           // runs at the same time
	Timeout   time.Duration
	CacheTTL  time.Duration
}

var DefaultLimits = Limits{Bits: 16, PerIP: 3, PerIPDay: 10, Window: 10 * time.Minute, BudgetEUR: 2,
	RunEUR: 0.03, Parallel: 3, Timeout: 50 * time.Second, CacheTTL: 6 * time.Hour}

// Service runs demos. Deps gives the bricks their services for one run's
// in-memory store; Chat is the AI.
type Service struct {
	Catalog *catalog.Catalog
	Deps    func(st store.Store) bricks.Deps
	Chat    llm.Chatter
	Model   string
	Secret  []byte // signs challenges
	Limits  Limits
	Log     *slog.Logger
	Now     func() time.Time

	once   sync.Once
	mu     sync.Mutex
	ips    map[string][]time.Time
	used   map[string]time.Time // spent challenge tokens
	day    string
	spent  float64
	cache  map[string]cached
	slots  chan struct{}
	runner func(ctx context.Context, in Input) (Result, float64, error) // tests
}

type cached struct {
	res Result
	at  time.Time
}

// Input is what the visitor typed or picked.
type Input struct {
	Template string
	Lang     string // en, de
	Business string // Morning Briefing only
	City     string
	Timezone string
	Token    string // from Challenge
	Nonce    string // proof of work
}

// Result is the Feed card the demo made, and the cards behind it (at most 3).
type Result struct {
	Template string
	Name     string
	Avatar   string
	Title    string
	Body     string
	Links    []Link
	Cards    []Card
	Cached   bool
	Seconds  float64
}

type Link struct{ Title, URL string }

type Card struct{ Title, Summary, URL string }

// Challenge is a signed token the browser must find a nonce for.
type Challenge struct {
	Token string
	Bits  int
}

func (s *Service) init() {
	s.once.Do(func() {
		if s.Limits == (Limits{}) {
			s.Limits = DefaultLimits
		}
		if len(s.Secret) == 0 {
			s.Secret = make([]byte, 32)
			_, _ = rand.Read(s.Secret)
		}
		if s.Log == nil {
			s.Log = slog.New(slog.DiscardHandler)
		}
		s.ips, s.used, s.cache = map[string][]time.Time{}, map[string]time.Time{}, map[string]cached{}
		s.slots = make(chan struct{}, max(1, s.Limits.Parallel))
		if s.runner == nil {
			s.runner = s.run
		}
	})
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// NewChallenge returns a token valid for 10 minutes and one run.
func (s *Service) NewChallenge() Challenge {
	s.init()
	r := make([]byte, 12)
	_, _ = rand.Read(r)
	payload := strconv.FormatInt(s.now().Unix(), 10) + "." + hex.EncodeToString(r)
	return Challenge{Token: payload + "." + s.sign(payload), Bits: s.Limits.Bits}
}

func (s *Service) sign(payload string) string {
	m := hmac.New(sha256.New, s.Secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// checkProof verifies the token and nonce and spends the token.
func (s *Service) checkProof(token, nonce string) error {
	i := strings.LastIndexByte(token, '.')
	if i < 0 || len(nonce) == 0 || len(nonce) > 20 || !hmac.Equal([]byte(token[i+1:]), []byte(s.sign(token[:i]))) {
		return ErrProof
	}
	ts, _, _ := strings.Cut(token, ".")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || s.now().Sub(time.Unix(sec, 0)) > 10*time.Minute {
		return ErrProof
	}
	sum := sha256.Sum256([]byte(token + ":" + nonce))
	if leadingZeros(sum[:]) < s.Limits.Bits {
		return ErrProof
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for t, at := range s.used {
		if s.now().Sub(at) > 11*time.Minute {
			delete(s.used, t)
		}
	}
	if _, spent := s.used[token]; spent {
		return ErrProof
	}
	s.used[token] = s.now()
	return nil
}

func leadingZeros(b []byte) int {
	n := 0
	for _, c := range b {
		if c != 0 {
			return n + bits.LeadingZeros8(c)
		}
		n += 8
	}
	return n
}

// Plain words only: they end up in a search query, so no operators, links
// or long prompts.
var plain = regexp.MustCompile(`^[\p{L}\p{M}0-9][\p{L}\p{M}0-9 .,'’&-]*$`)

func clean(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	return s, n >= 2 && n <= 60 && plain.MatchString(s) && !strings.Contains(s, "..")
}

func (s *Service) validate(in *Input) error {
	if !slices.Contains(Templates, in.Template) {
		return fmt.Errorf("%w: this helper can't be tried without an account", ErrInput)
	}
	if in.Lang != "de" {
		in.Lang = "en"
	}
	var ok bool
	if in.City, ok = clean(in.City); !ok {
		return fmt.Errorf("%w: city must be 2 to 60 letters", ErrInput)
	}
	if in.Template == "morning-briefing" {
		if in.Business, ok = clean(in.Business); !ok {
			return fmt.Errorf("%w: business must be 2 to 60 letters", ErrInput)
		}
	} else {
		in.Business = ""
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" || len(in.Timezone) > 64 {
		in.Timezone = "Europe/Berlin"
	}
	return nil
}

func (in Input) key() string {
	return strings.ToLower(strings.Join([]string{in.Template, in.Lang, in.Business, in.City}, "|"))
}

// Run checks the proof and the limits, then runs the demo (or serves it
// from the cache). ip is the caller's address.
func (s *Service) Run(ctx context.Context, ip string, in Input) (Result, error) {
	s.init()
	if err := s.validate(&in); err != nil {
		return Result{}, err
	}
	if err := s.checkProof(in.Token, in.Nonce); err != nil {
		return Result{}, err
	}
	if res, ok := s.cached(in.key()); ok {
		return res, nil
	}
	if err := s.admit(ipBucket(ip)); err != nil {
		return Result{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-time.After(5 * time.Second):
		return Result{}, ErrLimit
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, s.Limits.Timeout)
	defer cancel()
	start := s.now()
	res, cost, err := s.runner(ctx, in)
	s.spend(cost)
	res.Seconds = s.now().Sub(start).Seconds()
	s.Log.Info("demo run", "template", in.Template, "lang", in.Lang, "cost_eur", cost,
		"seconds", res.Seconds, "cards", len(res.Cards), "err", err)
	if err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	if len(s.cache) > 500 {
		clear(s.cache)
	}
	s.cache[in.key()] = cached{res: res, at: s.now()}
	s.mu.Unlock()
	return res, nil
}

func (s *Service) cached(key string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[key]
	if !ok || s.now().Sub(c.at) > s.Limits.CacheTTL {
		return Result{}, false
	}
	c.res.Cached = true
	return c.res, true
}

// admit counts a run against the IP and the daily budget.
func (s *Service) admit(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if d := now.UTC().Format(time.DateOnly); d != s.day {
		s.day, s.spent = d, 0
		clear(s.ips)
	}
	if s.spent >= s.Limits.BudgetEUR {
		return ErrBudget
	}
	recent := 0
	for _, t := range s.ips[ip] {
		if now.Sub(t) < s.Limits.Window {
			recent++
		}
	}
	if recent >= s.Limits.PerIP || len(s.ips[ip]) >= s.Limits.PerIPDay {
		return ErrLimit
	}
	s.ips[ip] = append(s.ips[ip], now)
	return nil
}

func (s *Service) spend(eur float64) {
	s.mu.Lock()
	s.spent += eur
	s.mu.Unlock()
}

// ipBucket groups IPv6 addresses by /64 (one home or phone gets many).
func ipBucket(ip string) string {
	if strings.Count(ip, ":") >= 2 {
		parts := strings.Split(ip, ":")
		if len(parts) >= 4 {
			return strings.Join(parts[:4], ":") + "::/64"
		}
	}
	return ip
}

// run installs the template in a throwaway in-memory workspace and runs it
// once in Test mode.
func (s *Service) run(ctx context.Context, in Input) (Result, float64, error) {
	t, ok := shelf.Find(in.Template)
	if !ok {
		return Result{}, 0, fmt.Errorf("%w: unknown template", ErrInput)
	}
	st := memory.New()
	user := store.NewID()
	ws := st.PutWorkspace(store.Workspace{UserID: user, Name: "Demo", Timezone: in.Timezone, Locale: in.Lang,
		Profile: map[string]any{}, Plan: "free"})
	st.PutBrain(store.Brain{WorkspaceID: ws.ID, UserID: user, AllowanceCredits: 50})

	answers := map[string]string{"business": in.Business, "city": in.City}
	for _, q := range t.SetupQuestions {
		if q.Type == "place" {
			answers[q.Key] = shelf.FeedOnly
		}
	}
	stack, profile, err := shelf.Install(s.Catalog, t, ws, answers, func(string) (store.Place, error) {
		return store.Place{}, store.ErrNotFound
	})
	if err != nil {
		return Result{}, 0, fmt.Errorf("%w: %w", ErrInput, err)
	}
	ws.Profile = profile
	ws = st.PutWorkspace(ws)
	// Look and think only: nothing waits for an answer, nothing is sent.
	stack.Bricks = slices.DeleteFunc(stack.Bricks, func(b catalog.StackBrick) bool {
		return strings.HasPrefix(b.Type, "ask.") || strings.HasPrefix(b.Type, "do.")
	})
	if stack.Limits == nil {
		stack.Limits = &catalog.Limits{}
	}
	stack.Limits.MaxCostEUR = &s.Limits.RunEUR
	stack.Limits.MaxExternalEmails = new(0)
	raw, err := json.Marshal(stack)
	if err != nil {
		return Result{}, 0, err
	}
	name := t.Name.In(in.Lang)
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: user, Name: name, Avatar: &t.Avatar, Status: "active"})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: user, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	h = st.PutHelper(h)

	eng := &engine.Engine{Store: st, Catalog: s.Catalog, Bricks: bricks.All(s.Deps(st)),
		Brain: &llm.Brain{Chat: s.Chat, Model: s.Model, Store: st}, Blobs: &engine.MemoryBlobs{},
		Resolve: tmpl.Config, Log: s.Log.With("demo", in.Template)}
	r, err := eng.StartRun(ctx, engine.StartRequest{HelperID: h.ID, TriggerType: "test"})
	if err != nil {
		return Result{}, 0, err
	}
	runErr := eng.Execute(ctx, r.ID)
	r, err = st.GetRun(ctx, r.ID)
	if err != nil {
		return Result{}, 0, err
	}
	cost := float64(r.CostEUR)
	if runErr != nil || r.Status != store.RunDone {
		return Result{}, cost, fmt.Errorf("demo run %s: %w", r.Status, runErr)
	}
	feed, err := st.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: ws.ID, Types: []string{"result"}})
	if err != nil {
		return Result{}, cost, err
	}
	if len(feed) == 0 {
		return Result{}, cost, errors.New("demo run: no result card")
	}
	fc := feed[0]
	res := Result{Template: in.Template, Name: name, Avatar: t.Avatar, Title: fc.Title}
	if fc.Body != nil {
		res.Body = *fc.Body
	}
	if links, ok := fc.Payload["links"].([]any); ok {
		for _, l := range links {
			if m, ok := l.(map[string]any); ok {
				res.Links = append(res.Links, Link{Title: str(m, "title"), URL: str(m, "url")})
			}
		}
	}
	if fc.StepID != nil {
		cards, _ := st.ListCards(ctx, *fc.StepID)
		for _, c := range cards[:min(engine.TestModeMaxCards, len(cards))] {
			card := Card{Title: str(c.Data, "title", "name"), Summary: str(c.Data, "summary", "description", "body"),
				URL: str(c.Data, "url", "website")}
			if card.URL == "" && c.SourceURL != nil {
				card.URL = *c.SourceURL
			}
			res.Cards = append(res.Cards, card)
		}
	}
	return res, cost, nil
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

package bricks_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	qmem "github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// #21: a retried or reclaimed job never sends twice, and a run whose job
// runs out of attempts ends with a Problem card. These run the real worker,
// engine and Post to chat brick together.

type countingSender struct {
	mu   sync.Mutex
	sent []string
}

func (s *countingSender) Send(_ context.Context, _, _ string, m chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m.Text)
	return nil
}

func (s *countingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

// crashyStore fails saving the pile of one step once: the worker "crashes"
// after Post to chat has sent, before the step is recorded as done.
type crashyStore struct {
	*memory.Store
	mu      sync.Mutex
	failFor string // brick type
	steps   map[string]string
	failed  bool
}

func (s *crashyStore) CreateStep(ctx context.Context, st *store.Step) error {
	err := s.Store.CreateStep(ctx, st)
	s.mu.Lock()
	s.steps[st.ID] = st.BrickType
	s.mu.Unlock()
	return err
}

func (s *crashyStore) InsertCards(ctx context.Context, cards []store.Card) error {
	s.mu.Lock()
	crash := !s.failed && len(cards) > 0 && s.steps[cards[0].StepID] == s.failFor
	if crash {
		s.failed = true
	}
	s.mu.Unlock()
	if crash {
		return errors.New("connection reset")
	}
	return s.Store.InsertCards(ctx, cards)
}

type rig struct {
	st     *crashyStore
	q      *qmem.Queue
	eng    *engine.Engine
	sender *countingSender
	runID  string
}

func newRig(t *testing.T, bricks map[string]engine.Brick) *rig {
	t.Helper()
	ms := memory.New()
	st := &crashyStore{Store: ms, steps: map[string]string{}, failFor: "do.chat"}
	user := store.NewID()
	ws := ms.PutWorkspace(store.Workspace{UserID: user, Name: "Bäckerei", Locale: "en"})
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	keys, _ := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(k), "")
	place := store.Place{ID: store.NewID(), WorkspaceID: ws.ID, UserID: user, Type: "discord", Label: "#news"}
	sealed, _ := keys.Encrypt([]byte("https://discord.com/api/webhooks/1/x"), crypto.AAD("places", place.ID, "secret"))
	place.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := ms.CreatePlace(context.Background(), &place); err != nil {
		t.Fatal(err)
	}
	h := ms.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: user, Name: "Briefing", Status: "active"})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.tap"},
		{ID: "b2", Type: "look.search", Config: map[string]any{"query": "bakery news"}},
		{ID: "b3", Type: "do.chat", Config: map[string]any{"place": place.ID, "mode": "each"}},
	}})
	v := ms.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: user, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	ms.PutHelper(h)

	sender := &countingSender{}
	all := map[string]engine.Brick{
		"when.tap": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
			return nil, nil
		}),
		"look.search": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
			var out []engine.Card
			for _, title := range []string{"Flour prices", "Tram line", "Market fees"} {
				var c engine.Card
				c.Set("title", title, engine.TrustUntrusted)
				out = append(out, c)
			}
			return out, nil
		}),
		"do.chat": do.Chat{Store: st, Keys: keys, Sender: sender},
	}
	for k, b := range bricks {
		all[k] = b
	}
	eng := &engine.Engine{Store: st, Catalog: catalog.Default(), Bricks: all, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r, err := eng.StartRun(context.Background(), engine.StartRequest{HelperID: h.ID, TriggerType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{st: st, q: qmem.New(), eng: eng, sender: sender, runID: r.ID}
}

// work runs a worker until the run's job is done or failed.
func (g *rig) work(t *testing.T, maxAttempts int) queue.Job {
	t.Helper()
	job, err := g.q.Enqueue(context.Background(), queue.NewJob{Kind: engine.JobKind, Payload: map[string]string{"run_id": g.runID}, MaxAttempts: maxAttempts})
	if err != nil {
		t.Fatal(err)
	}
	w := &queue.Worker{Queue: g.q, ID: "w1", Handlers: map[string]queue.Handler{engine.JobKind: g.eng.HandleJob},
		Poll: 2 * time.Millisecond, Backoff: func(int) time.Duration { return 0 },
		OnFailed: func(ctx context.Context, j queue.Job, cause error) { g.eng.JobFailed(ctx, j, cause) },
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range g.q.Jobs() {
			if j.ID == job.ID && (j.Status == "done" || j.Status == "failed") {
				return j
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("job didn't finish")
	return job
}

func TestRetriedRunNeverSendsTwice(t *testing.T) {
	g := newRig(t, nil)
	j := g.work(t, 5)
	if j.Status != "done" || j.Attempts != 2 {
		t.Fatalf("job %s after %d attempts", j.Status, j.Attempts)
	}
	r, _ := g.st.GetRun(context.Background(), g.runID)
	if r.Status != store.RunDone {
		t.Fatalf("run %s", r.Status)
	}
	if g.sender.count() != 3 {
		t.Fatalf("sent %d messages, want 3 (one per card, none twice): %v", g.sender.count(), g.sender.sent)
	}
	steps, _ := g.st.ListSteps(context.Background(), g.runID)
	var chatAttempts []string
	for _, s := range steps {
		if s.BrickType == "do.chat" {
			chatAttempts = append(chatAttempts, s.Status)
		}
	}
	if len(chatAttempts) != 2 || chatAttempts[0] != "failed" || chatAttempts[1] != "done" {
		t.Fatalf("Post to chat attempts: %v", chatAttempts)
	}
}

func TestRunOutOfAttemptsGetsAProblemCard(t *testing.T) {
	g := newRig(t, map[string]engine.Brick{
		"look.search": engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
			return nil, errors.New("search timed out")
		}),
	})
	j := g.work(t, 3)
	if j.Status != "failed" || j.Attempts != 3 {
		t.Fatalf("job %s after %d attempts", j.Status, j.Attempts)
	}
	r, _ := g.st.GetRun(context.Background(), g.runID)
	if r.Status != store.RunFailed {
		t.Fatalf("run %s", r.Status)
	}
	var problems int
	for _, f := range g.st.FeedCards() {
		if f.RunID != nil && *f.RunID == g.runID && f.Type == "problem" {
			problems++
		}
	}
	if problems != 1 || g.sender.count() != 0 {
		t.Fatalf("%d Problem cards, %d messages", problems, g.sender.count())
	}
}

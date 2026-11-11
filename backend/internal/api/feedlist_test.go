package api_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/realtime"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type feedEnv struct {
	env
	ws     store.Workspace
	helper store.Helper
	events chan realtime.Event
}

func newFeedEnv(t *testing.T) feedEnv {
	t.Helper()
	st := memory.New()
	events := make(chan realtime.Event, 8)
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: config.Config{}, Store: st,
		Auth: fakeAuth{"anna": anna, "ben": ben}, Events: api.NewHub(events)}).Handler())
	t.Cleanup(srv.Close)
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	st.PutBrain(store.Brain{WorkspaceID: ws.ID, UserID: anna.ID, Mode: "included", AllowanceCredits: 500,
		UsedCreditsThisPeriod: 120, PeriodStart: time.Now()})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Morning Briefing", Avatar: new("📰"), Status: "active"})
	return feedEnv{env: env{url: srv.URL, store: st}, ws: ws, helper: h, events: events}
}

func (e feedEnv) card(t *testing.T, typ, state, title string) store.FeedCard {
	t.Helper()
	c := store.FeedCard{WorkspaceID: e.ws.ID, UserID: anna.ID, HelperID: &e.helper.ID, Type: typ, State: state, Title: title}
	if err := e.store.CreateFeedCard(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func wsID(id string) openapi_types.UUID {
	var u openapi_types.UUID
	_ = u.UnmarshalText([]byte(id))
	return u
}

func TestListFeed(t *testing.T) {
	e := newFeedEnv(t)
	ctx := context.Background()
	e.card(t, "result", "open", "Done")
	ask := e.card(t, "approval", "open", "Check 3")
	e.card(t, "problem", "dismissed", "Old problem")
	e.card(t, "question", "answered", "Which city?")
	last := e.card(t, "result", "open", "5 new cards")
	c := e.client(t, "anna")

	page := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), nil))
	if page.StatusCode() != 200 {
		t.Fatalf("%d %s", page.StatusCode(), page.Body)
	}
	p := page.JSON200
	if len(p.Cards) != 4 || p.Cards[0].Id.String() != last.ID || p.NeedsMe != 1 || p.NextBefore != nil {
		t.Fatalf("all: %d cards, needs %d", len(p.Cards), p.NeedsMe)
	}
	if p.Cards[0].Helper == nil || p.Cards[0].Helper.Name != "Morning Briefing" || *p.Cards[0].Helper.Avatar != "📰" {
		t.Fatalf("helper: %+v", p.Cards[0].Helper)
	}
	if p.Brain.CreditsLeft != 380 || p.Brain.AllowanceCredits != 500 {
		t.Fatalf("brain: %+v", p.Brain)
	}

	needs := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), &api.ListFeedParams{Filter: new(api.NeedsMe)})).JSON200
	if len(needs.Cards) != 1 || needs.Cards[0].Id.String() != ask.ID {
		t.Fatalf("needs me: %+v", needs.Cards)
	}
	results := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), &api.ListFeedParams{Filter: new(api.Results)})).JSON200
	if len(results.Cards) != 2 {
		t.Fatalf("results: %d", len(results.Cards))
	}

	// Paging.
	first := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), &api.ListFeedParams{Limit: new(2)})).JSON200
	if len(first.Cards) != 2 || first.NextBefore == nil {
		t.Fatalf("page 1: %d, next %v", len(first.Cards), first.NextBefore)
	}
	second := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), &api.ListFeedParams{Limit: new(2), Before: first.NextBefore})).JSON200
	if len(second.Cards) != 2 || second.Cards[0].Id == first.Cards[1].Id || second.NextBefore != nil {
		t.Fatalf("page 2: %+v", second.Cards)
	}

	if res := must(e.client(t, "ben").ListFeedWithResponse(ctx, wsID(e.ws.ID), nil)); res.StatusCode() != 404 {
		t.Fatalf("ben: %d", res.StatusCode())
	}
}

func TestDismissFeedCard(t *testing.T) {
	e := newFeedEnv(t)
	ctx := context.Background()
	res := e.card(t, "result", "open", "Done")
	ask := e.card(t, "approval", "open", "Check 3")
	c := e.client(t, "anna")
	if r := must(c.DismissFeedCardWithResponse(ctx, wsID(ask.ID))); r.StatusCode() != http.StatusConflict {
		t.Fatalf("open approval: %d", r.StatusCode())
	}
	r := must(c.DismissFeedCardWithResponse(ctx, wsID(res.ID)))
	if r.StatusCode() != 200 || r.JSON200.State != "dismissed" {
		t.Fatalf("dismiss: %d %s", r.StatusCode(), r.Body)
	}
	if again := must(c.DismissFeedCardWithResponse(ctx, wsID(res.ID))); again.StatusCode() != 200 {
		t.Fatalf("twice: %d", again.StatusCode())
	}
	if page := must(c.ListFeedWithResponse(ctx, wsID(e.ws.ID), nil)).JSON200; len(page.Cards) != 1 {
		t.Fatalf("dismissed card still listed: %d", len(page.Cards))
	}
	if r := must(e.client(t, "ben").DismissFeedCardWithResponse(ctx, wsID(ask.ID))); r.StatusCode() != 404 {
		t.Fatalf("ben: %d", r.StatusCode())
	}
}

// Members get their workspace's events as SSE; other workspaces' events
// and anything without a workspace never arrive.
func TestEvents(t *testing.T) {
	e := newFeedEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.url+"/api/workspaces/"+e.ws.ID+"/events", nil)
	req.Header.Set("Authorization", "Bearer anna")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewReader(resp.Body)
	next := func() string { // the next event: and data: lines
		var ev []string
		for {
			l, err := lines.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			l = strings.TrimSpace(l)
			if l == "" && len(ev) > 0 {
				return strings.Join(ev, " ")
			}
			if l != "" && !strings.HasPrefix(l, ":") {
				ev = append(ev, l)
			}
		}
	}
	time.Sleep(20 * time.Millisecond) // subscribed
	e.events <- realtime.Event{Channel: "db:feed_cards", Type: "INSERT", Record: map[string]any{"id": "other", "workspace_id": store.NewID()}}
	e.events <- realtime.Event{Channel: "db:feed_cards", Type: "INSERT", Record: map[string]any{"id": "f1", "workspace_id": e.ws.ID, "type": "approval", "state": "open", "title": "secret stuff"}}
	if got := next(); got != `event: feed data: {"id":"f1","type":"approval","state":"open"}` {
		t.Fatalf("got %q", got)
	}
	e.events <- realtime.Event{Channel: "db:runs", Type: "UPDATE", Record: map[string]any{"id": "r1", "workspace_id": e.ws.ID, "status": "waiting"}}
	if got := next(); got != `event: run data: {"id":"r1","status":"waiting"}` {
		t.Fatalf("got %q", got)
	}
	e.events <- realtime.Event{Type: realtime.Reconnected}
	if got := next(); got != `event: reload data: {}` {
		t.Fatalf("got %q", got)
	}

	// Ben isn't a member.
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.url+"/api/workspaces/"+e.ws.ID+"/events", nil)
	req2.Header.Set("Authorization", "Bearer ben")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("ben: %d", resp2.StatusCode)
	}
}

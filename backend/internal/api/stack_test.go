package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func TestBuilderSave(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "en", Timezone: "Europe/Berlin",
		Profile: map[string]any{"business": "Bakery", "city": "Leipzig"}})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	other := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Other", Locale: "en", Timezone: "Europe/Berlin"})
	elsewhere := store.Place{ID: store.NewID(), WorkspaceID: other.ID, UserID: anna.ID, Type: "discord", Label: "Not yours"}
	if err := st.CreatePlace(ctx, &elsewhere); err != nil {
		t.Fatal(err)
	}
	c := e.client(t, "anna")
	h := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "morning-briefing",
		Answers: map[string]string{"chat": "feed"}})).JSON201.Helper

	got := must(c.GetHelperStackWithResponse(ctx, h.Id))
	if got.StatusCode() != 200 || got.JSON200.Version != 1 || got.JSON200.Stack.Bricks[0].Type != "when.every" {
		t.Fatalf("load: %d %s", got.StatusCode(), got.Body)
	}
	stack := got.JSON200.Stack

	// Move the time: version 2, and the schedule follows.
	stack.Bricks[0].Config["time"] = "06:15"
	saved := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: stack, Version: 1}))
	if saved.StatusCode() != 200 || saved.JSON200.Version != 2 || saved.JSON200.Stack.Bricks[0].Config["time"] != "06:15" {
		t.Fatalf("save: %d %s", saved.StatusCode(), saved.Body)
	}
	trs, _ := st.HelperTriggers(ctx, h.Id.String())
	if len(trs) != 1 || trs[0].Config["time"] != "06:15" {
		t.Fatalf("trigger %+v", trs)
	}
	if again := must(c.GetHelperStackWithResponse(ctx, h.Id)); again.JSON200.Version != 2 {
		t.Fatalf("current is v%d", again.JSON200.Version)
	}

	// Saved on top of an old version: refused.
	if r := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: stack, Version: 1})); r.StatusCode() != 409 {
		t.Fatalf("stale: %d", r.StatusCode())
	}
	// A When brick that isn't on top: problems, nothing saved.
	bad := stack
	bad.Bricks = append(append([]api.StackBrick{}, stack.Bricks[1:]...), stack.Bricks[0])
	if r := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: bad, Version: 2})); r.StatusCode() != 422 || len(r.JSON422.Issues) == 0 {
		t.Fatalf("bad order: %d %s", r.StatusCode(), r.Body)
	}
	// Someone else's chat: refused.
	chat := stack
	chat.Bricks = append(append([]api.StackBrick{}, stack.Bricks...),
		api.StackBrick{Id: "b9", Type: "do.chat", Config: map[string]any{"place": elsewhere.ID}})
	r := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: chat, Version: 2}))
	if r.StatusCode() != 422 || r.JSON422.Issues[len(r.JSON422.Issues)-1].Code != "bad_value" {
		t.Fatalf("foreign chat: %d %s", r.StatusCode(), r.Body)
	}
	if v := must(c.GetHelperStackWithResponse(ctx, h.Id)); v.JSON200.Version != 2 {
		t.Fatalf("saved anyway: v%d", v.JSON200.Version)
	}
}

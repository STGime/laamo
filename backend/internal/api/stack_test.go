package api_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	qmem "github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func TestBuilderSave(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}, Queue: qmem.New()}).Handler())
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
	// Someone else's list: refused too.
	theirs := store.List{WorkspaceID: other.ID, UserID: anna.ID, Name: "Theirs"}
	_ = st.CreateList(ctx, &theirs)
	toList := stack
	toList.Bricks = append(append([]api.StackBrick{}, stack.Bricks...),
		api.StackBrick{Id: "b9", Type: "do.list", Config: map[string]any{"list": theirs.ID, "dedupe": "none"}})
	if r := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: toList, Version: 2})); r.StatusCode() != 422 ||
		!strings.Contains(string(r.Body), "pick one of your lists") {
		t.Fatalf("foreign list: %d %s", r.StatusCode(), r.Body)
	}
	if v := must(c.GetHelperStackWithResponse(ctx, h.Id)); v.JSON200.Version != 2 {
		t.Fatalf("saved anyway: v%d", v.JSON200.Version)
	}

	// Test an edit (#42): it runs as its own version; the helper keeps v2.
	stack.Bricks[0].Config["time"] = "21:00"
	if r := must(c.TestHelperStackWithResponse(ctx, h.Id, api.TestStack{Stack: bad})); r.StatusCode() != 422 {
		t.Fatalf("bad test: %d", r.StatusCode())
	}
	tr := must(c.TestHelperStackWithResponse(ctx, h.Id, api.TestStack{Stack: stack}))
	if tr.StatusCode() != 202 {
		t.Fatalf("test: %d %s", tr.StatusCode(), tr.Body)
	}
	run, _ := st.GetRun(ctx, tr.JSON202.RunId.String())
	tested, _ := st.GetHelperVersion(ctx, run.VersionID)
	if !run.TestMode || tested.Version != 3 || !strings.Contains(string(tested.Stack), "21:00") {
		t.Fatalf("run %+v on v%d", run, tested.Version)
	}
	if v := must(c.GetHelperStackWithResponse(ctx, h.Id)); v.JSON200.Version != 2 {
		t.Fatalf("test made v%d current", v.JSON200.Version)
	}
	if r := must(c.TestHelperStackWithResponse(ctx, h.Id, api.TestStack{Stack: stack})); r.StatusCode() != 409 {
		t.Fatalf("second test while the first runs: %d", r.StatusCode())
	}
	// Saving after a test: the base is still v2.
	if r := must(c.SaveHelperStackWithResponse(ctx, h.Id, api.SaveStack{Stack: stack, Version: 2})); r.StatusCode() != 200 || r.JSON200.Version != 4 {
		t.Fatalf("save after test: %d %s", r.StatusCode(), r.Body)
	}
}

// "Build your own": a helper with just When I tap, ready for the builder.
func TestCreateBlank(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")
	for _, want := range []string{"Mein Helfer", "Mein Helfer (2)"} {
		r := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "blank", Answers: map[string]string{}}))
		if r.StatusCode() != 201 || r.JSON201.Helper.Name != want {
			t.Fatalf("blank: %d %s", r.StatusCode(), r.Body)
		}
		got := must(c.GetHelperStackWithResponse(ctx, r.JSON201.Helper.Id))
		if len(got.JSON200.Stack.Bricks) != 1 || got.JSON200.Stack.Bricks[0].Type != "when.tap" {
			t.Fatalf("stack %+v", got.JSON200.Stack)
		}
	}
}

// "Use in a helper" on a list: Read my list comes after When I tap.
func TestCreateFromList(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "en", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	l := store.List{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Leads"}
	_ = st.CreateList(ctx, &l)
	c := e.client(t, "anna")
	r := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "blank", Answers: map[string]string{"list": l.ID}}))
	if r.StatusCode() != 201 || r.JSON201.Helper.Name != "Leads" {
		t.Fatalf("from list: %d %s", r.StatusCode(), r.Body)
	}
	got := must(c.GetHelperStackWithResponse(ctx, r.JSON201.Helper.Id)).JSON200.Stack.Bricks
	if len(got) != 2 || got[1].Type != "look.my_stuff" || got[1].Config["list"] != l.ID || got[1].Config["source"] != "list" {
		t.Fatalf("stack %+v", got)
	}
	theirs := store.List{WorkspaceID: store.NewID(), UserID: anna.ID, Name: "Theirs"}
	_ = st.CreateList(ctx, &theirs)
	if r := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "blank", Answers: map[string]string{"list": theirs.ID}})); r.StatusCode() != 400 {
		t.Fatalf("their list: %d", r.StatusCode())
	}
}

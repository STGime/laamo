package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func TestLists(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "en", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")

	created := must(c.CreateListWithResponse(ctx, wsID(ws.ID), api.NewList{Name: "  Leads "}))
	if created.StatusCode() != 201 || created.JSON201.Name != "Leads" || created.JSON201.Rows != 0 {
		t.Fatalf("create: %d %s", created.StatusCode(), created.Body)
	}
	l := created.JSON201
	if r := must(c.CreateListWithResponse(ctx, wsID(ws.ID), api.NewList{Name: "Leads"})); r.StatusCode() != 409 {
		t.Fatalf("same name: %d", r.StatusCode())
	}
	clock := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, d := range []map[string]any{
		{"name": "Café Kuchenzeit", "email": "hallo@kuchenzeit.de", "rating": 4.5},
		{"name": "eiscafé Venezia", "note": "=HYPERLINK(\"evil\")"},
		{"name": "Bäckerei Ost", "rating": 3.0},
	} {
		clock = clock.Add(time.Hour)
		r := store.ListRow{WorkspaceID: ws.ID, UserID: anna.ID, ListID: l.Id.String(), Data: d, CreatedAt: clock}
		if err := st.AddListRow(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	cols := []map[string]any{{"key": "name"}, {"key": "email"}, {"key": "rating"}, {"key": "note"}}
	_, _ = st.UpdateList(ctx, l.Id.String(), store.ListPatch{Columns: &cols})

	// Newest first by default; search; sort by a column.
	got := must(c.GetListWithResponse(ctx, l.Id, nil)).JSON200
	if got.Matching != 3 || got.Rows[0].Data["name"] != "Bäckerei Ost" || len(got.Columns) != 4 || got.List.Rows != 3 {
		t.Fatalf("list %+v", got)
	}
	q, sortBy, asc := "CAFÉ", "name", api.Asc
	got = must(c.GetListWithResponse(ctx, l.Id, &api.GetListParams{Q: &q, Sort: &sortBy, Order: &asc})).JSON200
	if got.Matching != 2 || got.Rows[0].Data["name"] != "Café Kuchenzeit" {
		t.Fatalf("search+sort %+v", got.Rows)
	}
	sortBy = "rating"
	got = must(c.GetListWithResponse(ctx, l.Id, &api.GetListParams{Sort: &sortBy, Order: &asc})).JSON200
	if got.Rows[0].Data["name"] != "Bäckerei Ost" || got.Rows[2].Data["name"] != "eiscafé Venezia" { // empty last
		t.Fatalf("by rating %v", got.Rows)
	}

	// CSV: header, BOM, formulas defused.
	res := must(c.ExportListWithResponse(ctx, l.Id))
	body := string(res.Body)
	if res.StatusCode() != 200 || !strings.HasPrefix(body, "\ufeffname,email,rating,note,saved_at\n") || !strings.Contains(body, `"'=HYPERLINK(""evil"")"`) {
		t.Fatalf("csv %d:\n%s", res.StatusCode(), body)
	}

	// Someone else sees nothing.
	b := e.client(t, "ben")
	if r := must(b.GetListWithResponse(ctx, l.Id, nil)); r.StatusCode() != 404 {
		t.Fatalf("ben: %d", r.StatusCode())
	}

	// Share: a public read-only link, until it's turned off.
	shared := must(c.ShareListWithResponse(ctx, l.Id)).JSON200
	if shared.ShareToken == nil || len(*shared.ShareToken) < 30 {
		t.Fatalf("share %+v", shared)
	}
	public := func() (int, string) { // no sign-in
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+api.BaseURL+"/shared/lists/"+*shared.ShareToken, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := public(); code != 200 || !strings.Contains(body, "Kuchenzeit") {
		t.Fatalf("public: %d %s", code, body)
	}
	if r := must(c.UnshareListWithResponse(ctx, l.Id)); r.StatusCode() != 204 {
		t.Fatalf("unshare %d", r.StatusCode())
	}
	if code, _ := public(); code != 404 {
		t.Fatalf("after unshare: %d", code)
	}

	// Delete a row, rename, delete the list.
	if r := must(c.DeleteListRowWithResponse(ctx, l.Id, got.Rows[0].Id)); r.StatusCode() != 204 {
		t.Fatalf("delete row %d", r.StatusCode())
	}
	if r := must(c.UpdateListWithResponse(ctx, l.Id, api.NewList{Name: "Cafés"})); r.StatusCode() != 200 || r.JSON200.Rows != 2 {
		t.Fatalf("rename %d %s", r.StatusCode(), r.Body)
	}
	if ls := must(c.ListListsWithResponse(ctx, wsID(ws.ID))).JSON200; len(*ls) != 1 || (*ls)[0].Name != "Cafés" {
		t.Fatalf("lists %+v", ls)
	}
	if r := must(c.DeleteListWithResponse(ctx, l.Id)); r.StatusCode() != 204 {
		t.Fatalf("delete %d", r.StatusCode())
	}
	if n, _ := st.CountListRows(ctx, l.Id.String()); n != 0 {
		t.Fatalf("rows left: %d", n)
	}
}

// "Add to list" from the Feed: picked cards from a result become rows;
// duplicates are skipped; cards come from the run, not the request.
func TestAddToListFromFeed(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "en", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")

	runID, stepID := store.NewID(), store.NewID()
	var cards []store.Card
	var ids []string
	for i, title := range []string{"Flour prices rise", "New hygiene rules", "Christmas market fees"} {
		id := store.NewID()
		ids = append(ids, id)
		cards = append(cards, store.Card{WorkspaceID: ws.ID, UserID: anna.ID, RunID: runID, StepID: stepID, CardID: id,
			Position: i, Kind: "web_result", Data: map[string]any{"title": title, "url": "https://news.example/" + id, "page_text": "long"}})
	}
	if err := st.InsertCards(ctx, cards); err != nil {
		t.Fatal(err)
	}
	fc := store.FeedCard{WorkspaceID: ws.ID, UserID: anna.ID, RunID: &runID, StepID: &stepID, Type: "result", Title: "3 new cards"}
	if err := st.CreateFeedCard(ctx, &fc); err != nil {
		t.Fatal(err)
	}
	l := must(c.CreateListWithResponse(ctx, wsID(ws.ID), api.NewList{Name: "Read later"})).JSON201

	pick := []openapi_types.UUID{wsID(ids[0]), wsID(ids[2])}
	r := must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(fc.ID), CardIds: &pick}))
	if r.StatusCode() != 200 || r.JSON200.Added != 2 || r.JSON200.Duplicates != 0 || r.JSON200.List.Rows != 2 {
		t.Fatalf("add: %d %s", r.StatusCode(), r.Body)
	}
	rows, _ := st.ListRows(ctx, l.Id.String(), 0)
	for _, row := range rows {
		if row.Data["page_text"] != nil || row.Data["url"] == nil {
			t.Fatalf("row %v", row.Data)
		}
	}
	// All of them: two are there already.
	r = must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(fc.ID)}))
	if r.JSON200.Added != 1 || r.JSON200.Duplicates != 2 {
		t.Fatalf("all: %s", r.Body)
	}
	// A card that isn't on this Feed card.
	bad := []openapi_types.UUID{wsID(store.NewID())}
	if r := must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(fc.ID), CardIds: &bad})); r.StatusCode() != 400 {
		t.Fatalf("unknown card: %d", r.StatusCode())
	}
	// A summary's source links (Morning Briefing): chosen by their url.
	sum := store.FeedCard{WorkspaceID: ws.ID, UserID: anna.ID, RunID: &runID, Type: "result", Title: "Done",
		Payload: map[string]any{"links": []any{
			map[string]any{"title": "Metzger-Ranking", "url": "https://www.rbb24.de/metzger"},
			map[string]any{"title": "Insolvenz", "url": "https://bild.de/insolvenz"},
			map[string]any{"title": "Bad", "url": "javascript:alert(1)"},
		}}}
	_ = st.CreateFeedCard(ctx, &sum)
	links := []string{"https://www.rbb24.de/metzger"}
	r = must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(sum.ID), Links: &links}))
	if r.StatusCode() != 200 || r.JSON200.Added != 1 {
		t.Fatalf("link: %d %s", r.StatusCode(), r.Body)
	}
	if row, _ := st.GetListRowByKey(ctx, l.Id.String(), "url:https://rbb24.de/metzger"); row.Data["title"] != "Metzger-Ranking" {
		t.Fatalf("link row %+v", row)
	}
	links = []string{"https://evil.example/not-on-the-card"}
	if r := must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(sum.ID), Links: &links})); r.StatusCode() != 400 {
		t.Fatalf("made-up link: %d", r.StatusCode())
	}
	// No step, no cards: everything means its (safe) links.
	r = must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(sum.ID)}))
	if r.JSON200.Added != 1 || r.JSON200.Duplicates != 1 {
		t.Fatalf("all links: %s", r.Body)
	}

	// The summary card itself (as an older app asked for it): its links, not its text.
	sumStep := store.NewID()
	_ = st.InsertCards(ctx, []store.Card{{WorkspaceID: ws.ID, UserID: anna.ID, RunID: runID, StepID: sumStep, CardID: store.NewID(),
		Kind: "summary", Data: map[string]any{"summary": "Butchers ranked…", "sources": []any{
			map[string]any{"title": "Metzger-Ranking", "url": "https://www.rbb24.de/metzger"},
			map[string]any{"title": "Neu", "url": "https://example.org/neu"},
		}}}})
	sum2 := store.FeedCard{WorkspaceID: ws.ID, UserID: anna.ID, RunID: &runID, StepID: &sumStep, Type: "result", Title: "Done"}
	_ = st.CreateFeedCard(ctx, &sum2)
	r = must(c.AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(sum2.ID)}))
	if r.JSON200.Added != 1 || r.JSON200.Duplicates != 1 {
		t.Fatalf("summary card: %s", r.Body)
	}
	if row, err := st.GetListRowByKey(ctx, l.Id.String(), "url:https://example.org/neu"); err != nil || row.Data["summary"] != nil {
		t.Fatalf("summary row %+v %v", row, err)
	}

	// Someone else can't add to it.
	if r := must(e.client(t, "ben").AddListRowsWithResponse(ctx, l.Id, api.AddListRows{FeedCardId: wsID(fc.ID)})); r.StatusCode() != 404 {
		t.Fatalf("ben: %d", r.StatusCode())
	}
}

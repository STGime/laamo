package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

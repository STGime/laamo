package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// The export for bookkeeping: the CSV and the receipt photos in one file,
// with the CSV naming each row's picture.
func TestExportListZip(t *testing.T) {
	st := memory.New()
	blobs := &engine.MemoryBlobs{}
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}, Blobs: blobs}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")
	l := must(c.CreateListWithResponse(ctx, wsID(ws.ID), api.NewList{Name: "Belege 2026"})).JSON201

	mine := "images/" + ws.ID + "/run-1/p1.jpg"
	other := "images/" + store.NewID() + "/run-9/p1.jpg" // another workspace's picture
	_ = blobs.Put(ctx, mine, []byte("JPEG of the slip"))
	_ = blobs.Put(ctx, other, []byte("not yours"))
	clock := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for _, d := range []map[string]any{
		{"vendor": "Café Sonne", "date": "2026-11-14", "total": "11.50", "image": mine},
		{"vendor": "Taxi Müller", "date": "2026-11-15", "total": "23.00"},
		{"vendor": "Fremd GmbH", "total": "1.00", "image": other},
	} {
		clock = clock.Add(time.Hour)
		r := store.ListRow{WorkspaceID: ws.ID, UserID: anna.ID, ListID: l.Id.String(), Data: d, CreatedAt: clock}
		if err := st.AddListRow(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	cols := []map[string]any{{"key": "vendor"}, {"key": "date"}, {"key": "total"}, {"key": "image"}}
	_, _ = st.UpdateList(ctx, l.Id.String(), store.ListPatch{Columns: &cols})

	res := must(c.ExportListZipWithResponse(ctx, l.Id))
	if res.StatusCode() != 200 || res.HTTPResponse.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("zip: %d %s", res.StatusCode(), res.HTTPResponse.Header.Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	csv := files["belege-2026.csv"]
	var picture string
	for name := range files {
		if strings.HasPrefix(name, "pictures/") {
			picture = name
		}
	}
	// One picture: this workspace's. Named after the vendor and date, and
	// the CSV row points at it; the other rows have no picture.
	if len(files) != 2 || files[picture] != "JPEG of the slip" ||
		!strings.HasSuffix(picture, "-cafe-sonne-2026-11-14.jpg") {
		t.Fatalf("files: %v", keys(files))
	}
	if !strings.Contains(csv, "Café Sonne,2026-11-14,11.50,"+picture) ||
		!strings.Contains(csv, "Taxi Müller,2026-11-15,23.00,,") || !strings.Contains(csv, "Fremd GmbH,,1.00,,") ||
		strings.Contains(csv, "images/") {
		t.Fatalf("csv:\n%s", csv)
	}

	// Members only.
	if r := must(e.client(t, "ben").ExportListZipWithResponse(ctx, l.Id)); r.StatusCode() != 404 {
		t.Fatalf("a stranger's export: %d", r.StatusCode())
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

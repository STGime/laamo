package do_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func lead(id, name, website string) engine.Card {
	c := engine.Card{ID: id, Kind: "place"}
	c.Set("name", name, engine.TrustExtracted)
	if website != "" {
		c.Set("website", website, engine.TrustExtracted)
	}
	c.Set("page_text", strings.Repeat("long page ", 100), engine.TrustUntrusted)
	c.Set("custom_note", "Has a terrace", engine.TrustExtracted)
	return c
}

func TestSaveToList(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	rc := brickstest.RunContext("do.list")
	l := store.List{WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, Name: "Leads"}
	if err := st.CreateList(ctx, &l); err != nil {
		t.Fatal(err)
	}
	b := do.List{Store: st}
	cfg := map[string]any{"list": l.ID, "dedupe": "website"}
	in := []engine.Card{
		lead("c1", "Café Kuchenzeit", "https://www.kuchenzeit.de/"),
		lead("c2", "Kuchenzeit again", "kuchenzeit.de"), // same website
		lead("c3", "Eiscafé Venezia", ""),               // no website: saved anyway
	}
	out, err := brickstest.Contract(t, "do.list", b, rc, in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := st.ListRows(ctx, l.ID, 0)
	if len(rows) != 2 || out[0].Text("list_row_id") == "" || out[1].Text("list_row_id") != out[0].Text("list_row_id") {
		t.Fatalf("%d rows; ids %q %q", len(rows), out[0].Text("list_row_id"), out[1].Text("list_row_id"))
	}
	row, _ := st.GetListRow(ctx, out[0].Text("list_row_id"))
	if row.Data["name"] != "Café Kuchenzeit" || row.Data["page_text"] != nil || row.Data["custom_note"] != "Has a terrace" ||
		*row.DedupeKey != "website:kuchenzeit.de" || *row.SourceRunID != rc.Run.ID {
		t.Fatalf("row %+v", row)
	}
	got, _ := st.GetList(ctx, l.ID)
	if fmt.Sprint(got.Columns) != "[map[key:name] map[key:website] map[key:custom_note]]" {
		t.Fatalf("columns %v", got.Columns)
	}

	// A retried job finds its own rows: nothing new, same ids.
	again, err := brickstest.Contract(t, "do.list", b, rc, []engine.Card{lead("c1", "Café Kuchenzeit", "kuchenzeit.de"), lead("c3", "Eiscafé Venezia", "")}, cfg)
	if err != nil || again[1].Text("list_row_id") != out[2].Text("list_row_id") {
		t.Fatalf("retry: %v %q", err, again[1].Text("list_row_id"))
	}
	if n, _ := st.CountListRows(ctx, l.ID); n != 2 {
		t.Fatalf("retry added rows: %d", n)
	}

	// Another workspace's list: refused.
	other := store.List{WorkspaceID: store.NewID(), UserID: rc.Run.UserID, Name: "Theirs"}
	_ = st.CreateList(ctx, &other)
	_, err = b.Run(ctx, rc, in, engine.Config{"list": other.ID, "dedupe": "none"})
	if !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("foreign list: %v", err)
	}
}

func TestSaveToListFull(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	rc := brickstest.RunContext("do.list")
	l := store.List{WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, Name: "Big"}
	_ = st.CreateList(ctx, &l)
	for i := range do.MaxListRows {
		_ = st.AddListRow(ctx, &store.ListRow{WorkspaceID: l.WorkspaceID, UserID: l.UserID, ListID: l.ID, Data: map[string]any{"n": i}})
	}
	_, err := do.List{Store: st}.Run(ctx, rc, []engine.Card{lead("c1", "New", "new.de")}, engine.Config{"list": l.ID, "dedupe": "website"})
	if err == nil || !strings.Contains(err.Error(), "is full") {
		t.Fatalf("full list: %v", err)
	}
}

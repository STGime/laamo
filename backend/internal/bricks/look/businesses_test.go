package look_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/bricks/look"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/osm"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type fakeMap struct {
	places map[string]osm.Place
	got    []osm.Query
}

func (f *fakeMap) Geocode(_ context.Context, q, _ string) (osm.Place, error) {
	p, ok := f.places[q]
	if !ok {
		return osm.Place{}, osm.ErrNoPlace
	}
	return p, nil
}

func (f *fakeMap) Find(_ context.Context, q osm.Query) ([]osm.Business, error) {
	f.got = append(f.got, q)
	return []osm.Business{
		{OSMURL: "https://www.openstreetmap.org/way/1", Name: "Bäckerei Klein", Address: "Hauptstraße 5, 04109 Leipzig",
			Website: "https://baeckerei-klein.de", Phone: "+49 341 123", Email: "info@klein.de", OpeningHours: "Mo-Fr 06:00-18:00",
			Lat: 51.3400, Lon: 12.3735, DistanceM: 345},
		{OSMURL: "https://www.openstreetmap.org/node/2", Name: "Backstube Ost"},
	}, nil
}

type tagModel struct{ reply string }

func (m tagModel) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Text: m.reply, Model: "test", TokensIn: 10, TokensOut: 5}, nil
}

func TestFindBusinesses(t *testing.T) {
	m := &fakeMap{places: map[string]osm.Place{"Leipzig": {Name: "Leipzig", Lat: 51.34, Lon: 12.37}}}
	b := look.FindBusinesses{Finder: m}
	rc := brickstest.RunContext("look.find_businesses")
	out, err := brickstest.Contract(t, "look.find_businesses", b, rc, nil,
		map[string]any{"what": "Bäckereien", "where": "{my city}", "radius_km": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	q := m.got[0]
	if len(q.Tags) != 1 || q.Tags[0] != (osm.Tag{Key: "shop", Value: "bakery"}) || q.RadiusM != 5000 || q.Lat != 51.34 || q.Limit != 50 {
		t.Fatalf("query %+v", q)
	}
	c := out[0]
	if len(out) != 2 || c.Kind != "business" || c.Source != "https://www.openstreetmap.org/way/1" || c.Text("website") != "https://baeckerei-klein.de" ||
		c.Trust["name"] != engine.TrustExtracted || c.Text("email") != "" || out[1].Text("phone") != "" ||
		c.Text("location") != "51.340000,12.373500" || c.Fields["distance_km"] != 0.35 || c.Text("distance_from") != "Leipzig" || out[1].Text("location") != "" {
		t.Fatalf("cards %+v", out)
	}

	// Not in the list: the AI picks tags (only allowed ones).
	rc = brickstest.RunContext("look.find_businesses")
	st := memory.New()
	st.PutBrain(store.Brain{WorkspaceID: rc.Run.WorkspaceID, AllowanceCredits: 500})
	rc.Brain = &llm.Brain{Chat: tagModel{`{"tags":[{"key":"craft","value":"beekeeper"}]}`}, Store: st, USDToEUR: 1}
	if _, err := b.Run(context.Background(), rc, nil, engine.Config{"what": "Imker", "where": "Leipzig", "radius_km": float64(10)}); err != nil {
		t.Fatal(err)
	}
	if q := m.got[1]; len(q.Tags) != 1 || q.Tags[0].Value != "beekeeper" {
		t.Fatalf("ai query %+v", q)
	}

	// No AI, unknown kind: by name.
	rc = brickstest.RunContext("look.find_businesses")
	if _, err := b.Run(context.Background(), rc, nil, engine.Config{"what": "Kuchenzeit", "where": "Leipzig", "radius_km": float64(10)}); err != nil {
		t.Fatal(err)
	}
	if q := m.got[2]; len(q.Tags) != 0 || q.Name != "Kuchenzeit" {
		t.Fatalf("name query %+v", q)
	}

	// A place the map doesn't know: a clear message, no retries.
	_, err = b.Run(context.Background(), rc, nil, engine.Config{"what": "bakery", "where": "Atlantis", "radius_km": float64(10)})
	if !errors.Is(err, queue.ErrPermanent) || !strings.Contains(err.Error(), "Atlantis") {
		t.Fatalf("unknown place: %v", err)
	}
}

// regionMap knows regions and countries with ids, and pages through 25
// businesses per place.
type regionMap struct{ got []osm.Query }

func (m *regionMap) Geocode(_ context.Context, q, _ string) (osm.Place, error) {
	ids := map[string]osm.Place{
		"Deutschland": {Name: "Deutschland", ID: "aaaaaaaaaaaaaaaaaaaa01", Type: "country", Lat: 51, Lon: 10},
		"Österreich":  {Name: "Österreich", ID: "aaaaaaaaaaaaaaaaaaaa02", Type: "country", Lat: 47.5, Lon: 14.5},
		"Schweiz":     {Name: "Schweiz", ID: "aaaaaaaaaaaaaaaaaaaa03", Type: "country", Lat: 46.8, Lon: 8.2},
		"Sachsen":     {Name: "Sachsen", ID: "aaaaaaaaaaaaaaaaaaaa04", Type: "state", Lat: 51, Lon: 13.3},
		"Leipzig":     {Name: "Leipzig", ID: "aaaaaaaaaaaaaaaaaaaa05", Type: "city", Lat: 51.34, Lon: 12.37},
	}
	p, ok := ids[q]
	if !ok {
		return osm.Place{}, osm.ErrNoPlace
	}
	return p, nil
}

func (m *regionMap) Find(_ context.Context, q osm.Query) ([]osm.Business, error) {
	m.got = append(m.got, q)
	id := "c0" + q.PlaceID // around a point: "c0"
	var out []osm.Business
	for i := q.Offset; i < min(25, q.Offset+q.Limit); i++ {
		out = append(out, osm.Business{Name: fmt.Sprintf("Café %s %d", id[len(id)-2:], i),
			Address: fmt.Sprintf("Weg %d", i), Lat: 50, Lon: 10, DistanceM: float64(i * 1000)})
	}
	return out, nil
}

func TestFindBusinessesAreas(t *testing.T) {
	ctx := context.Background()
	m := &regionMap{}
	st := memory.New()
	b := look.FindBusinesses{Finder: m, Lists: st}
	rc := brickstest.RunContext("look.find_businesses")
	rc.Limits.MaxCards = 12

	// Several countries: their whole areas, in turn, no distances.
	out, err := b.Run(ctx, rc, nil, engine.Config{"what": "Cafés", "where": "DACH", "radius_km": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	countries := map[string]int{}
	for _, c := range out {
		countries[strings.Fields(c.Text("name"))[1]]++
		if c.Text("distance_km") != "" || c.Text("distance_from") != "" {
			t.Fatalf("distance in an area search: %+v", c.Fields)
		}
	}
	if len(out) != 12 || countries["01"] != 4 || countries["02"] != 4 || countries["03"] != 4 || m.got[0].PlaceID == "" {
		t.Fatalf("%d cards %v, first query %+v", len(out), countries, m.got[0])
	}

	// A region on its own: its whole area too.
	m.got = nil
	if _, err := b.Run(ctx, rc, nil, engine.Config{"what": "Cafés", "where": "Sachsen", "radius_km": float64(10)}); err != nil {
		t.Fatal(err)
	}
	if m.got[0].PlaceID != "aaaaaaaaaaaaaaaaaaaa04" {
		t.Fatalf("region query %+v", m.got[0])
	}

	// A city: around it, with distances, unless "the whole place".
	m.got = nil
	out, _ = b.Run(ctx, rc, nil, engine.Config{"what": "Cafés", "where": "Leipzig", "radius_km": float64(5)})
	if m.got[0].PlaceID != "" || m.got[0].RadiusM != 5000 || out[0].Text("distance_from") != "Leipzig" {
		t.Fatalf("city query %+v", m.got[0])
	}
	m.got = nil
	_, _ = b.Run(ctx, rc, nil, engine.Config{"what": "Cafés", "where": "Leipzig", "radius_km": "whole"})
	if m.got[0].PlaceID != "aaaaaaaaaaaaaaaaaaaa05" {
		t.Fatalf("whole city query %+v", m.got[0])
	}

	// Skip what's in my list: next week brings the next batch.
	l := store.List{WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, Name: "Leads"}
	_ = st.CreateList(ctx, &l)
	cfg := engine.Config{"what": "Cafés", "where": "Sachsen", "radius_km": float64(10), "skip_list": l.ID}
	week1, _ := b.Run(ctx, rc, nil, cfg)
	for _, c := range week1 {
		key := do.DedupeKey("name", c.Text("name"))
		_ = st.AddListRow(ctx, &store.ListRow{WorkspaceID: l.WorkspaceID, UserID: l.UserID, ListID: l.ID, DedupeKey: &key,
			Data: map[string]any{"name": c.Text("name")}})
	}
	week2, _ := b.Run(ctx, rc, nil, cfg)
	if len(week1) != 12 || len(week2) != 12 || week2[0].Text("name") != "Café 04 12" {
		t.Fatalf("week 1 %d, week 2 %d starting %q", len(week1), len(week2), week2[0].Text("name"))
	}
	week2Rows := week2
	for _, c := range week2Rows {
		key := do.DedupeKey("name", c.Text("name"))
		_ = st.AddListRow(ctx, &store.ListRow{WorkspaceID: l.WorkspaceID, UserID: l.UserID, ListID: l.ID, DedupeKey: &key,
			Data: map[string]any{"name": c.Text("name")}})
	}
	if week3, _ := b.Run(ctx, rc, nil, cfg); len(week3) != 1 {
		t.Fatalf("week 3: %d (25 in Sachsen, 24 seen)", len(week3))
	}

	// A place the map doesn't know, among known ones: the known ones still run.
	if out, err := b.Run(ctx, rc, nil, engine.Config{"what": "Cafés", "where": "Atlantis; Leipzig", "radius_km": float64(5)}); err != nil || len(out) == 0 {
		t.Fatalf("partly unknown: %v %d", err, len(out))
	}
}

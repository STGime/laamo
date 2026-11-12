package look_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
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
			Website: "https://baeckerei-klein.de", Phone: "+49 341 123", Email: "info@klein.de", OpeningHours: "Mo-Fr 06:00-18:00"},
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
		c.Trust["name"] != engine.TrustExtracted || c.Text("email") != "" || out[1].Text("phone") != "" {
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

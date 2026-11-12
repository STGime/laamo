package osm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCategories(t *testing.T) {
	if got := Categories("Bäckereien"); len(got) != 1 || got[0] != "commercial.food_and_drink.bakery" {
		t.Fatalf("bakeries: %v", got)
	}
	if got := Categories("Klempner"); got != nil {
		t.Fatalf("plumbers have no category: %v", got)
	}
	if got := CategoriesFor([]Tag{{"office", "lawyer"}, {"craft", "beekeeper"}}); len(got) != 1 || got[0] != "office.lawyer" {
		t.Fatalf("for tags: %v", got)
	}
}

func TestGeoapify(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apiKey") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		calls = append(calls, r.URL.Path+" "+q.Get("categories")+" "+q.Get("name")+" "+q.Get("filter"))
		switch r.URL.Path {
		case "/v1/geocode/search":
			_, _ = w.Write([]byte(`{"results":[{"formatted":"Leipzig, SN, Deutschland","lat":51.34,"lon":12.37}]}`))
		case "/v1/staticmap":
			if !strings.Contains(q.Get("marker"), "lonlat:12.37000,51.34000") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("JPEG"))
		case "/v2/places":
			_, _ = w.Write([]byte(`{"features":[
				{"properties":{"name":"Far","distance":900,"datasource":{"sourcename":"openstreetmap","raw":{"osm_type":"n","osm_id":7}}}},
				{"properties":{"name":"Ditsch","street":"Neumarkt","housenumber":"1","postcode":"04109","city":"Leipzig",
					"website":"https://www.ditsch.de/","opening_hours":"Mo-Sa 11:00-20:00","distance":208,
					"datasource":{"sourcename":"openstreetmap","raw":{"osm_type":"w","osm_id":310218381,"phone":"+49 341 1"}}}},
				{"properties":{"distance":5}}]}`))
		}
	}))
	defer srv.Close()
	g := &Geoapify{APIKey: "k3y", BaseURL: srv.URL}
	ctx := context.Background()
	p, err := g.Geocode(ctx, "Leipzig", "de")
	if err != nil || p.Lat != 51.34 {
		t.Fatalf("geocode %+v %v", p, err)
	}
	bs, err := g.Find(ctx, Query{Categories: Categories("bakery"), Name: "bakery", Lat: p.Lat, Lon: p.Lon, RadiusM: 2000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[0].Name != "Ditsch" || bs[0].Address != "Neumarkt 1, 04109 Leipzig" || bs[0].Phone != "+49 341 1" ||
		bs[0].OSMURL != "https://www.openstreetmap.org/way/310218381" || bs[1].OSMURL != "https://www.openstreetmap.org/node/7" {
		t.Fatalf("businesses %+v", bs)
	}
	if !strings.Contains(calls[1], "commercial.food_and_drink.bakery") || !strings.Contains(calls[1], "circle:12.370000,51.340000,2000") {
		t.Fatalf("query %q", calls[1])
	}
	// Unknown kind: by name, in the broad categories.
	if _, err := g.Find(ctx, Query{Name: "Klempner Müller", Lat: 1, Lon: 2, RadiusM: 1000}); err != nil {
		t.Fatal(err)
	}
	if c := calls[2]; !strings.Contains(c, "commercial,catering,service") || !strings.Contains(c, "Klempner Müller") {
		t.Fatalf("name query %q", c)
	}
	// A small map: fetched once, then kept.
	g.MapsURL = srv.URL
	for range 2 {
		if img, err := g.StaticMap(ctx, 51.34, 12.37); err != nil || string(img) != "JPEG" {
			t.Fatalf("map %q %v", img, err)
		}
	}
	if _, err := g.StaticMap(ctx, 123, 0); err == nil {
		t.Fatal("bad location accepted")
	}
	calls = calls[:len(calls)-1] // the one map call

	// Cached: no new call.
	_, _ = g.Geocode(ctx, "Leipzig", "de")
	if len(calls) != 3 {
		t.Fatalf("%d calls", len(calls))
	}
	// A bad key: a clear error that doesn't show the key.
	bad := &Geoapify{APIKey: "wrong-secret", BaseURL: srv.URL}
	if _, err := bad.Geocode(ctx, "Berlin", "de"); err == nil || strings.Contains(err.Error(), "wrong-secret") || errors.Is(err, ErrBusy) {
		t.Fatalf("bad key: %v", err)
	}
	// Unreachable: busy (retry later), and the key isn't in the message.
	down := &Geoapify{APIKey: "s3cret", BaseURL: "http://127.0.0.1:1"}
	if _, err := down.Geocode(ctx, "Berlin", "de"); !errors.Is(err, ErrBusy) || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("down: %v", err)
	}
}

// TestLiveGeoapify: two real requests (of 3000 free a day). task test:live.
func TestLiveGeoapify(t *testing.T) {
	key := os.Getenv("GEOAPIFY_API_KEY")
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || key == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and GEOAPIFY_API_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g := &Geoapify{APIKey: key}
	p, err := g.Geocode(ctx, "Leipzig", "de")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := g.Find(ctx, Query{Categories: Categories("Friseure"), Lat: p.Lat, Lon: p.Lon, RadiusM: 2000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) == 0 {
		t.Fatal("no hairdressers in central Leipzig?")
	}
	for _, b := range bs {
		t.Logf("%4.0f m  %s | %s | %s | %s | %s", b.DistanceM, b.Name, b.Address, b.Website, b.Phone, b.OSMURL)
	}
}

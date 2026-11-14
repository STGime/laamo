package osm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestKinds(t *testing.T) {
	for what, want := range map[string]string{
		"Bäckereien": "shop=bakery", "bakeries": "shop=bakery", "Bakery": "shop=bakery",
		"Friseure": "shop=hairdresser", "hair salons": "shop=hairdresser", "Cafés": "amenity=cafe",
		"Metzgereien": "shop=butcher", "Apotheken": "amenity=pharmacy", "Steuerberater": "office=tax_advisor",
		"Zahnärzte": "", "space agencies": "",
	} {
		got := Kinds(what)
		s := ""
		if len(got) > 0 {
			s = got[0].Key + "=" + got[0].Value
		}
		if s != want {
			t.Errorf("%q: %q, want %q", what, s, want)
		}
	}
	for _, k := range kinds {
		for _, tag := range k.tags {
			if !ValidTag(tag) {
				t.Errorf("bad tag %+v", tag)
			}
		}
	}
}

func TestQuerySafe(t *testing.T) {
	if _, err := overpassQL(Query{Tags: []Tag{{"shop", `bakery"];out;`}}, RadiusM: 1000}); err == nil {
		t.Fatal("bad tag value accepted")
	}
	if _, err := overpassQL(Query{Tags: []Tag{{"name", "x"}}, RadiusM: 1000}); err == nil {
		t.Fatal("tag key outside the list accepted")
	}
	ql, err := overpassQL(Query{Name: `Kuchen"];node(1);out;("`, Lat: 51.34, Lon: 12.37, RadiusM: 5000, Limit: 10})
	if err != nil || strings.Contains(ql, `"]`+";node") || !strings.Contains(ql, `"name"~"Kuchen.*node.*1.*out",i`) {
		t.Fatalf("name query: %v %s", err, ql)
	}
}

func TestFindAndGeocode(t *testing.T) {
	var nom, over atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/search":
			nom.Add(1)
			if r.URL.Query().Get("q") == "Nowhere" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"lat":"51.3397","lon":"12.3731","display_name":"Leipzig, Sachsen, Deutschland"}]`))
		case "/interpreter":
			over.Add(1)
			b, _ := io.ReadAll(r.Body)
			v, _ := url.ParseQuery(string(b))
			if !strings.Contains(v.Get("data"), `nwr["shop"="bakery"]["name"](around:2000,51.339700,12.373100);`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"elements":[
				{"type":"node","id":2,"lat":51.36,"lon":12.40,"tags":{"name":"Far Bakery","shop":"bakery"}},
				{"type":"way","id":1,"center":{"lat":51.3400,"lon":12.3735},"tags":{"name":"Bäckerei Klein","shop":"bakery",
					"addr:street":"Hauptstraße","addr:housenumber":"5","addr:postcode":"04109","addr:city":"Leipzig",
					"contact:website":"baeckerei-klein.de","phone":"+49 341 123","email":"Info@Klein.de;x@y.de","opening_hours":"Mo-Fr 06:00-18:00"}},
				{"type":"node","id":3,"lat":51.34,"lon":12.37,"tags":{"name":"Bäckerei Klein","shop":"bakery",
					"addr:street":"Hauptstraße","addr:housenumber":"5","addr:postcode":"04109","addr:city":"Leipzig"}},
				{"type":"node","id":4,"lat":51.34,"lon":12.37,"tags":{"shop":"bakery"}},
				{"type":"node","id":5,"lat":51.34,"lon":12.37,"tags":{"name":"Bad Site","website":"javascript:alert(1)"}}]}`))
		}
	}))
	defer srv.Close()
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer busy.Close()
	// The first Overpass server is busy: the second one answers.
	c := &Client{NominatimURL: srv.URL, OverpassURL: busy.URL + "," + srv.URL + "/interpreter"}
	ctx := context.Background()
	p, err := c.Geocode(ctx, "Leipzig", "de")
	if err != nil || p.Lat != 51.3397 {
		t.Fatalf("geocode %+v %v", p, err)
	}
	if _, err := c.Geocode(ctx, " leipzig ", "de"); err != nil || nom.Load() != 1 {
		t.Fatalf("cached geocode: %v, %d calls", err, nom.Load())
	}
	if _, err := c.Geocode(ctx, "Nowhere", "de"); !errors.Is(err, ErrNoPlace) {
		t.Fatalf("nowhere: %v", err)
	}
	q := Query{Tags: Kinds("Bäckereien"), Lat: p.Lat, Lon: p.Lon, RadiusM: 2000, Limit: 10}
	bs, err := c.Find(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 3 || bs[0].Name != "Bäckerei Klein" || bs[0].OSMURL != "https://www.openstreetmap.org/way/1" ||
		bs[0].Address != "Hauptstraße 5, 04109 Leipzig" || bs[0].Website != "https://baeckerei-klein.de" ||
		bs[0].Email != "info@klein.de" || bs[0].DistanceM > 100 || bs[2].Name != "Far Bakery" {
		t.Fatalf("businesses %+v", bs)
	}
	if bs[1].Website != "" {
		t.Fatalf("javascript: website kept: %+v", bs[1])
	}
	if _, err := c.Find(ctx, q); err != nil || over.Load() != 1 {
		t.Fatalf("cached find: %v, %d calls", err, over.Load())
	}
}

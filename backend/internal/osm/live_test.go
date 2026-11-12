package osm

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveFind: one Nominatim and one Overpass call on the public servers.
// task test:live.
func TestLiveFind(t *testing.T) {
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := &Client{}
	p, err := c.Geocode(ctx, "Leipzig", "de")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := c.Find(ctx, Query{Tags: Kinds("Bäckereien"), Lat: p.Lat, Lon: p.Lon, RadiusM: 2000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) == 0 {
		t.Fatal("no bakeries in central Leipzig?")
	}
	for _, b := range bs {
		t.Logf("%4.0f m  %s | %s | %s | %s", b.DistanceM, b.Name, b.Address, b.Website, b.Phone)
	}
}

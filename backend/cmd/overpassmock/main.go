// Command overpassmock is a local stand-in for the Overpass API (dev only).
// It answers every query with a few fixed businesses so Find businesses
// can be developed without hitting the public Overpass instance.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

const fixture = `{
  "version": 0.6,
  "generator": "laamo overpassmock",
  "elements": [
    {"type": "node", "id": 1, "lat": 48.137, "lon": 11.575,
     "tags": {"name": "Bäckerei Maier", "shop": "bakery", "website": "https://example.com/maier",
              "phone": "+49 89 000000", "addr:street": "Beispielstraße", "addr:housenumber": "1", "addr:city": "München"}},
    {"type": "node", "id": 2, "lat": 48.139, "lon": 11.580,
     "tags": {"name": "Brotzeit Huber", "shop": "bakery", "addr:city": "München"}},
    {"type": "node", "id": 3, "lat": 48.141, "lon": 11.570,
     "tags": {"name": "Café Sonne", "amenity": "cafe", "website": "https://example.com/sonne", "addr:city": "München"}}
  ]
}`

func main() {
	addr := ":8081"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/interpreter", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	})
	slog.Info("overpassmock listening", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

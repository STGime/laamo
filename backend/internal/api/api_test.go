package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
)

// newClient serves the API and returns the generated client pointed at it.
func newClient(t *testing.T, cfg config.Config) *api.ClientWithResponses {
	t.Helper()
	srv := httptest.NewServer(api.NewServer(cfg, nil).Handler())
	t.Cleanup(srv.Close)
	c, err := api.NewClientWithResponses(srv.URL + api.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Health round-trips through the generated client and server.
func TestHealthRoundTrip(t *testing.T) {
	eurobase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(eurobase.Close)

	cases := []struct {
		name string
		cfg  config.Config
		want api.HealthEurobase
	}{
		{"not configured", config.Config{Env: "dev"}, api.HealthEurobaseNotConfigured},
		{"reachable", config.Config{Env: "dev", EurobaseURL: eurobase.URL, EurobaseSecretKey: "eb_sk_x"}, api.HealthEurobaseReachable},
		{"unreachable", config.Config{Env: "dev", EurobaseURL: "http://127.0.0.1:1", EurobaseSecretKey: "eb_sk_x"}, api.HealthEurobaseUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := newClient(t, tc.cfg).GetHealthWithResponse(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
				t.Fatalf("status %d: %s", res.StatusCode(), res.Body)
			}
			if h := res.JSON200; h.Status != "ok" || h.Env != "dev" || h.Eurobase != tc.want {
				t.Fatalf("health: %+v", h)
			}
		})
	}
}

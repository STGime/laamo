package eurobase_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
)

// server answers with the given statuses in order, then 200 {"result": 1}.
func server(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "eb_sk_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		i := int(n.Add(1)) - 1
		if i < len(statuses) {
			if statuses[i] == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "2")
			}
			w.WriteHeader(statuses[i])
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result": 1, "id": "x"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func client(url string, waits *[]time.Duration) *eb.Client {
	return eb.New(url, "eb_sk_test", eb.WithSleep(func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	}))
}

func TestRetryPolicy(t *testing.T) {
	ctx := context.Background()
	rpc := func(c *eb.Client) error { return c.RPC(ctx, "claim_jobs", map[string]any{"worker": "w"}, nil) }
	get := func(c *eb.Client) error { var v any; return c.Get(ctx, "runs", "x", &v) }

	cases := []struct {
		name      string
		statuses  []int
		call      func(*eb.Client) error
		wantCalls int32
		wantErr   error // nil = success
	}{
		{"rpc retries 429", []int{429, 429}, rpc, 3, nil},
		{"rpc retries 503", []int{503}, rpc, 2, nil},
		{"rpc does not retry 502", []int{502}, rpc, 1, eb.ErrUncertain},
		{"rpc does not retry 504", []int{504}, rpc, 1, eb.ErrUncertain},
		{"rpc 500 is final", []int{500}, rpc, 1, errAny},
		{"get retries 502", []int{502, 504}, get, 3, nil},
		{"get 404", []int{404}, get, 1, eb.ErrNotFound},
		{"gives up after 5 tries", []int{503, 503, 503, 503, 503, 503}, rpc, 5, errAny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := server(t, tc.statuses...)
			var waits []time.Duration
			err := tc.call(client(srv.URL, &waits))
			if got := calls.Load(); got != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", got, tc.wantCalls)
			}
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case errors.Is(tc.wantErr, errAny) && err == nil:
				t.Fatal("want an error")
			case tc.wantErr != nil && !errors.Is(tc.wantErr, errAny) && !errors.Is(err, tc.wantErr):
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if errors.Is(tc.wantErr, errAny) && errors.Is(err, eb.ErrUncertain) {
				t.Fatalf("%v should not be uncertain", err)
			}
		})
	}
}

var errAny = errors.New("any error")

func TestRetryAfterAndBackoffBounds(t *testing.T) {
	srv, _ := server(t, 429, 503, 503, 503)
	var waits []time.Duration
	if err := client(srv.URL, &waits).RPC(context.Background(), "f", nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 4 || waits[0] != 2*time.Second {
		t.Fatalf("waits = %v, want Retry-After 2s first", waits)
	}
	for _, w := range waits[1:] {
		if w <= 0 || w > 5*time.Second {
			t.Fatalf("backoff out of bounds: %v", waits)
		}
	}
}

func TestBodyTooLarge(t *testing.T) {
	srv, calls := server(t)
	var waits []time.Duration
	big := make([]byte, eb.MaxBody)
	err := client(srv.URL, &waits).RPC(context.Background(), "f", map[string]any{"x": string(big)}, nil)
	if !errors.Is(err, eb.ErrTooLarge) || calls.Load() != 0 {
		t.Fatalf("want ErrTooLarge without a request, got %v (%d calls)", err, calls.Load())
	}
}

func TestCanceledContextStopsRetries(t *testing.T) {
	srv, calls := server(t, 503, 503, 503)
	ctx, cancel := context.WithCancel(context.Background())
	c := eb.New(srv.URL, "eb_sk_test", eb.WithSleep(func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}))
	if err := c.RPC(ctx, "f", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestQueryEncoding(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data": [], "count": 0}`))
	}))
	t.Cleanup(srv.Close)
	var waits []time.Duration
	q := eb.NewQuery().Eq("name", "Leads & Co.").In("status", "queued", "running").
		Order("created_at", true).Order("id", false).Limit(5)
	if _, err := client(srv.URL, &waits).List(context.Background(), "lists", q, nil); err != nil {
		t.Fatal(err)
	}
	want := "limit=5&name=eq.Leads+%26+Co.&order=created_at.desc%2Cid.asc&status=in.%28queued%2Crunning%29"
	if got != want {
		t.Fatalf("query\n got  %s\n want %s", got, want)
	}
}

// Eurobase (#829+) maps RPC database errors to a status plus the SQLSTATE.
func TestRPCErrorCodes(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantCalls int32
		conflict  bool
	}{
		{"duplicate key is a conflict", 400, `{"error":"duplicate key value","code":"23505"}`, 1, true},
		{"deadlock is retried", 409, `{"error":"deadlock detected","code":"40P01"}`, 2, false},
		{"serialization failure is retried", 409, `{"error":"could not serialize","code":"40001"}`, 2, false},
		{"REST unique violation is a conflict", 409, `{"error":"a record with this id already exists"}`, 1, true},
		{"function RAISE is final", 400, `{"error":"n must be between 1 and 100","code":"22023"}`, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var n atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if n.Add(1) == 1 {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				_, _ = w.Write([]byte(`{"result": []}`))
			}))
			t.Cleanup(srv.Close)
			var waits []time.Duration
			err := client(srv.URL, &waits).RPC(context.Background(), "f", nil, nil)
			if got := n.Load(); got != tc.wantCalls {
				t.Fatalf("calls = %d, want %d (err %v)", got, tc.wantCalls, err)
			}
			if tc.wantCalls == 2 && err != nil {
				t.Fatalf("retry should succeed: %v", err)
			}
			if got := errors.Is(err, eb.ErrConflict); got != tc.conflict {
				t.Fatalf("ErrConflict = %v, want %v (err %v)", got, tc.conflict, err)
			}
			var apiErr *eb.APIError
			if tc.wantCalls == 1 && (!errors.As(err, &apiErr) || (strings.Contains(tc.body, "code") && apiErr.Code == "")) {
				t.Fatalf("code not kept: %v", err)
			}
		})
	}
}

package eurobase_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
)

// realtimeServer mimics Eurobase realtime: checks token and Origin, acks the
// subscription, then sends events. The first connection is dropped after one
// event to exercise reconnecting.
func realtimeServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("token") != "eb_sk_test" {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Origin") != srv.URL {
			http.Error(w, "bad origin", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		n := conns.Add(1)
		ctx := r.Context()
		send := func(v any) {
			b, _ := json.Marshal(v)
			_ = c.Write(ctx, websocket.MessageText, b)
		}
		send(map[string]string{"type": "welcome", "project_id": "p"})
		_, msg, err := c.Read(ctx)
		if err != nil {
			return
		}
		var sub map[string]string
		_ = json.Unmarshal(msg, &sub)
		if sub["action"] != "subscribe" || sub["channel"] != "db:jobs" {
			send(map[string]string{"error": "unexpected subscribe " + string(msg)})
			return
		}
		send(map[string]string{"action": "subscribed", "channel": "db:jobs"})
		send(map[string]any{"channel": "db:jobs", "type": "UPDATE", "record": map[string]string{"id": "x"}})
		send(map[string]any{"channel": "db:jobs", "type": "INSERT", "record": map[string]string{"id": "j1"}})
		if n == 1 {
			return // drop the first connection
		}
		<-ctx.Done()
	}))
	t.Cleanup(srv.Close)
	return srv, &conns
}

func TestWakeupsSignalInsertsAndReconnect(t *testing.T) {
	srv, conns := realtimeServer(t)
	ctx := t.Context()
	ch := qeb.Wakeups(ctx, srv.URL, "eb_sk_test", quietLog())

	for i := 1; i <= 2; i++ {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("wake-up %d not received", i)
		}
	}
	if conns.Load() < 2 {
		t.Fatalf("expected a reconnect, got %d connections", conns.Load())
	}
}

func TestWakeupsBadKeyKeepsRetryingQuietly(t *testing.T) {
	srv, conns := realtimeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	ch := qeb.Wakeups(ctx, srv.URL, "eb_sk_wrong", quietLog())
	select {
	case <-ch:
		t.Fatal("no wake-up expected with a bad key")
	case <-ctx.Done():
	}
	if conns.Load() != 0 {
		t.Fatal("bad key must not connect")
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/realtime"
)

// Event is one live update for a workspace's members. It carries ids and
// states only; the app refetches through the API (where access is checked).
type Event struct {
	Kind   string `json:"-"` // feed, run, reload
	ID     string `json:"id,omitempty"`
	Type   string `json:"type,omitempty"`
	State  string `json:"state,omitempty"`
	Status string `json:"status,omitempty"`
	Helper string `json:"helper_id,omitempty"`
	// runs: the brick it's at; run_steps: which brick and its counts.
	Step     *int   `json:"step,omitempty"`
	Brick    string `json:"brick_id,omitempty"`
	CardsOut *int   `json:"cards_out,omitempty"`
}

// Hub fans Eurobase table changes out to the members of each workspace.
// One server-side realtime connection serves every browser: Eurobase
// realtime doesn't accept browser origins outside its own domain, and this
// way only members get their workspace's events.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{} // workspace id → subscribers
}

// NewHub starts dispatching events (from realtime.Subscribe on feed_cards
// and runs).
func NewHub(events <-chan realtime.Event) *Hub {
	h := &Hub{subs: map[string]map[chan Event]struct{}{}}
	go func() {
		for ev := range events {
			h.dispatch(ev)
		}
	}()
	return h
}

func (h *Hub) dispatch(ev realtime.Event) {
	if ev.Type == realtime.Reconnected {
		h.broadcast(Event{Kind: "reload"})
		return
	}
	ws := ev.String("workspace_id")
	if ws == "" {
		return
	}
	var out Event
	switch ev.Table() {
	case "feed_cards":
		out = Event{Kind: "feed", ID: ev.String("id"), Type: ev.String("type"), State: ev.String("state"), Helper: ev.String("helper_id")}
	case "runs":
		out = Event{Kind: "run", ID: ev.String("id"), Status: ev.String("status"), Helper: ev.String("helper_id"),
			Step: intField(ev, "next_brick_index")}
	case "run_steps":
		out = Event{Kind: "step", ID: ev.String("run_id"), Status: ev.String("status"), Brick: ev.String("brick_id"),
			CardsOut: intField(ev, "cards_out")}
	default:
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[ws] {
		select {
		case ch <- out:
		default: // a slow client gets a reload instead
			select {
			case ch <- Event{Kind: "reload"}:
			default:
			}
		}
	}
}

func (h *Hub) broadcast(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for ch := range set {
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func (h *Hub) subscribe(ws string) chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[ws] == nil {
		h.subs[ws] = map[chan Event]struct{}{}
	}
	h.subs[ws][ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(ws string, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[ws], ch)
	if len(h.subs[ws]) == 0 {
		delete(h.subs, ws)
	}
}

// heartbeat keeps proxies from closing an idle stream.
var heartbeat = 25 * time.Second

// serveEvents is GET /api/workspaces/{workspaceId}/events: Server-Sent
// Events for one workspace (members only).
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	_, m, ok, err := s.member(r.Context(), r.PathValue("workspaceId"))
	switch {
	case err != nil:
		WriteError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	case !ok:
		WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		return
	case s.events == nil:
		WriteError(w, http.StatusServiceUnavailable, "internal", "live updates are off")
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	if err := rc.Flush(); err != nil {
		return
	}
	_ = rc.SetWriteDeadline(time.Time{}) // a stream outlives the server's write timeout

	ch := s.events.subscribe(m.WorkspaceID)
	defer s.events.unsubscribe(m.WorkspaceID, ch)
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			_, err = fmt.Fprint(w, ": ping\n\n")
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, data)
		}
		if err == nil {
			err = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}

func intField(ev realtime.Event, field string) *int {
	if n, ok := ev.Record[field].(float64); ok {
		i := int(n)
		return &i
	}
	return nil
}

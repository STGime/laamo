package schedule_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/schedule"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// The same cases as migrations/test/queue_test.sql, so Go and SQL agree.
func TestNextMatchesSQL(t *testing.T) {
	cases := []struct {
		name string
		cfg  schedule.Config
		tz   string
		from string
		want string
	}{
		{"daily same day", schedule.Config{Freq: "daily", Time: "08:00"}, "Europe/Berlin", "2026-11-10T06:00:00Z", "2026-11-10T07:00:00Z"},
		{"exactly at the slot → next day", schedule.Config{Freq: "daily", Time: "08:00"}, "Europe/Berlin", "2026-11-10T07:00:00Z", "2026-11-11T07:00:00Z"},
		{"weekdays skip the weekend", schedule.Config{Freq: "weekdays", Time: "07:30"}, "Europe/Berlin", "2026-11-13T18:00:00Z", "2026-11-16T06:30:00Z"},
		{"weekly next monday", schedule.Config{Freq: "weekly", Days: []string{"mon"}, Time: "08:00"}, "Europe/Berlin", "2026-11-10T12:00:00Z", "2026-11-16T07:00:00Z"},
		{"weekly two days", schedule.Config{Freq: "weekly", Days: []string{"tue", "fri"}, Time: "10:00"}, "Europe/Berlin", "2026-11-10T11:00:00Z", "2026-11-13T09:00:00Z"},
		{"DST spring forward", schedule.Config{Freq: "daily", Time: "07:30"}, "Europe/Berlin", "2027-03-27T07:00:00Z", "2027-03-28T05:30:00Z"},
		{"weekly same day later", schedule.Config{Freq: "weekly", Days: []string{"tue"}, Time: "18:00"}, "Europe/Berlin", "2026-11-10T12:00:00Z", "2026-11-10T17:00:00Z"},
		{"weekly a week ahead", schedule.Config{Freq: "weekly", Days: []string{"tue"}, Time: "08:00"}, "Europe/Berlin", "2026-11-10T12:00:00Z", "2026-11-17T07:00:00Z"},
		{"local date differs from UTC", schedule.Config{Freq: "daily", Time: "00:30"}, "Pacific/Auckland", "2026-11-10T10:00:00Z", "2026-11-10T11:30:00Z"},
		{"inside the DST gap moves forward", schedule.Config{Freq: "daily", Time: "02:30"}, "Europe/Berlin", "2027-03-27T12:00:00Z", "2027-03-28T01:30:00Z"},
		{"defaults: daily 08:00", schedule.Config{}, "UTC", "2026-11-10T09:00:00Z", "2026-11-11T08:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schedule.Next(tc.cfg, tc.tz, at(tc.from))
			if err != nil || !got.Equal(at(tc.want)) {
				t.Fatalf("got %v %v, want %s", got, err, tc.want)
			}
		})
	}
}

func TestNextErrors(t *testing.T) {
	now := at("2026-11-10T12:00:00Z")
	if _, err := schedule.Next(schedule.Config{Freq: "weekly", Time: "08:00"}, "UTC", now); !errors.Is(err, schedule.ErrNever) {
		t.Fatalf("weekly without days: %v", err)
	}
	for name, c := range map[string]struct {
		cfg schedule.Config
		tz  string
	}{
		"hourly":     {schedule.Config{Freq: "hourly"}, "UTC"},
		"bad time":   {schedule.Config{Time: "8am"}, "UTC"},
		"bad zone":   {schedule.Config{}, "Mars/Olympus"},
		"local zone": {schedule.Config{}, "Local"},
		"empty zone": {schedule.Config{}, ""},
	} {
		if _, err := schedule.Next(c.cfg, c.tz, now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	cat := catalog.Default()
	ws := store.Workspace{ID: store.NewID(), Timezone: "Europe/Berlin"}
	h := store.Helper{ID: store.NewID(), WorkspaceID: ws.ID, UserID: store.NewID(), Status: "active"}
	now := at("2026-11-10T12:00:00Z") // a Tuesday
	every := func(cfg map[string]any) catalog.Stack {
		return catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{{ID: "b1", Type: "when.every", Config: cfg}}}
	}
	triggers := func() []store.Trigger {
		tr, err := st.HelperTriggers(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}

	// New helper with Every weekday 07:30 → one trigger, next Wednesday 06:30 UTC.
	if err := schedule.Sync(ctx, st, cat, h, ws, every(map[string]any{"freq": "weekdays", "time": "07:30"}), now); err != nil {
		t.Fatal(err)
	}
	tr := triggers()
	if len(tr) != 1 || tr[0].Type != store.TriggerSchedule || tr[0].NextRunAt == nil || !tr[0].NextRunAt.Equal(at("2026-11-11T06:30:00Z")) {
		t.Fatalf("created: %+v", tr)
	}
	if tr[0].Config["freq"] != "weekdays" || tr[0].Config["days"] != nil {
		t.Fatalf("config: %v", tr[0].Config)
	}

	// Changed to weekly Friday → same trigger updated.
	if err := schedule.Sync(ctx, st, cat, h, ws, every(map[string]any{"freq": "weekly", "days": []any{"fri"}, "time": "09:00"}), now); err != nil {
		t.Fatal(err)
	}
	tr2 := triggers()
	if len(tr2) != 1 || tr2[0].ID != tr[0].ID || !tr2[0].NextRunAt.Equal(at("2026-11-13T08:00:00Z")) {
		t.Fatalf("updated: %+v", tr2)
	}

	// Defaults from the catalog (weekdays 08:00).
	if err := schedule.Sync(ctx, st, cat, h, ws, every(nil), now); err != nil {
		t.Fatal(err)
	}
	if tr := triggers(); !tr[0].NextRunAt.Equal(at("2026-11-11T07:00:00Z")) {
		t.Fatalf("defaults: %v", tr[0].NextRunAt)
	}

	// Paused → trigger kept, no next run.
	paused := h
	paused.Status = "paused"
	if err := schedule.Sync(ctx, st, cat, paused, ws, every(nil), now); err != nil {
		t.Fatal(err)
	}
	if tr := triggers(); len(tr) != 1 || tr[0].NextRunAt != nil {
		t.Fatalf("paused: %+v", tr)
	}

	// Switched to Tap Go → schedule trigger removed, other triggers kept.
	addr := "x@in.laamo.app"
	other := store.Trigger{WorkspaceID: ws.ID, UserID: h.UserID, HelperID: h.ID, Type: store.TriggerEmail, InboundAddress: &addr}
	if err := st.CreateTrigger(ctx, &other); err != nil {
		t.Fatal(err)
	}
	tap := catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{{ID: "b1", Type: "when.tap"}}}
	if err := schedule.Sync(ctx, st, cat, h, ws, tap, now); err != nil {
		t.Fatal(err)
	}
	if tr := triggers(); len(tr) != 1 || tr[0].Type != store.TriggerEmail {
		t.Fatalf("after switching to Tap Go: %+v", tr)
	}

	// A bad workspace timezone is an error, not a silent no-schedule.
	bad := ws
	bad.Timezone = "Mars/Olympus"
	if err := schedule.Sync(ctx, st, cat, h, bad, every(nil), now); err == nil {
		t.Fatal("bad timezone accepted")
	}
}

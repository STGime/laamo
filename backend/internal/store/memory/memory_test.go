package memory_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
	"github.com/STGime/laamo/backend/internal/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) (store.Store, storetest.Fixture) {
		s := memory.New()
		s.SetClock(tickingClock())
		return s, Seed(s)
	})
}

// Seed puts one workspace, helper and version into s.
func Seed(s *memory.Store) storetest.Fixture {
	user := store.NewID()
	w := s.PutWorkspace(store.Workspace{UserID: user, Name: "Bäckerei Krause", Timezone: "Europe/Berlin"})
	h := s.PutHelper(store.Helper{WorkspaceID: w.ID, UserID: user, Name: "Morning Briefing", Status: "active"})
	v := s.PutHelperVersion(store.HelperVersion{WorkspaceID: w.ID, UserID: user, HelperID: h.ID, Version: 1,
		Stack: json.RawMessage(`{"bricks":[]}`)})
	return storetest.Fixture{WorkspaceID: w.ID, UserID: user, HelperID: h.ID, VersionID: v.ID, Timezone: w.Timezone}
}

// tickingClock advances by a millisecond per call, so created_at is unique.
func tickingClock() func() time.Time {
	var mu sync.Mutex
	t := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t = t.Add(time.Millisecond)
		return t
	}
}

func TestNewIDIsV7AndOrdered(t *testing.T) {
	a := store.NewID()
	time.Sleep(2 * time.Millisecond)
	b := store.NewID()
	if len(a) != 36 || a[14] != '7' {
		t.Fatalf("not a v7 uuid: %s", a)
	}
	if v := a[19]; v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Fatalf("wrong variant: %s", a)
	}
	if a >= b {
		t.Fatalf("ids not time ordered: %s >= %s", a, b)
	}
}

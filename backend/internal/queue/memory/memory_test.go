package memory_test

import (
	"sync"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/queue/queuetest"
)

// Clock is a settable test clock.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Microsecond) // strictly increasing
	return c.t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newQueue(t *testing.T) (queue.Queue, func(time.Duration)) {
	t.Helper()
	q := memory.New()
	c := &Clock{t: time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)}
	q.SetClock(c.Now)
	return q, c.Advance
}

func TestConformance(t *testing.T) { queuetest.Run(t, newQueue) }

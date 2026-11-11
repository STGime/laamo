package eurobase

import (
	"context"
	"log/slog"

	"github.com/STGime/laamo/backend/internal/realtime"
)

// Wakeups sends on the returned channel whenever a job is inserted through
// REST (Enqueue), and after a realtime reconnect (inserts may have been
// missed). It's only a hint for idle workers to claim now instead of at the
// next poll: cron-inserted jobs fire no event.
func Wakeups(ctx context.Context, baseURL, secretKey string, log *slog.Logger) <-chan struct{} {
	ch := make(chan struct{}, 1)
	events := realtime.Subscribe(ctx, baseURL, secretKey, []string{"jobs"}, log)
	go func() {
		for ev := range events {
			if ev.Type == realtime.Reconnected || (ev.Table() == "jobs" && ev.Type == "INSERT") {
				select {
				case ch <- struct{}{}:
				default: // a wake-up is already pending
				}
			}
		}
	}()
	return ch
}

// Command worker runs the job queue: run engine, outbound messages and digests.
//
// It claims jobs from the Eurobase jobs table (polling ~1/s when idle, woken
// early by realtime inserts) and stops cleanly on SIGINT/SIGTERM: jobs in
// flight are handed back to the queue.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/STGime/laamo/backend/internal/config"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/queue"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("worker exited", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.EurobaseSecretKey == "" {
		return fmt.Errorf("EUROBASE_PRIVATE_KEY is required for the worker")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	host, _ := os.Hostname()
	w := &queue.Worker{
		Queue:    qeb.New(eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)),
		ID:       fmt.Sprintf("%s-%d", host, os.Getpid()),
		Handlers: handlers(log),
		Wake:     qeb.Wakeups(ctx, cfg.EurobaseURL, cfg.EurobaseSecretKey, log),
		Log:      log,
	}
	return w.Run(ctx)
}

// handlers maps job kinds to handlers. "run" arrives with the engine (#18).
func handlers(log *slog.Logger) map[string]queue.Handler {
	return map[string]queue.Handler{
		// ping: smoke test for the queue (enqueue {"kind":"ping"} and watch it finish).
		"ping": func(_ context.Context, j queue.Job) error {
			log.Info("pong", "job", j.ID, "attempt", j.Attempts)
			return nil
		},
	}
}

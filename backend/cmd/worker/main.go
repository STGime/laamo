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

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/engine"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
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

	client := eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)
	st := sebs.New(client)
	if cfg.LLMAPIKey == "" {
		log.Warn("no LLM key (FAL_KEY, FAL_AI_API_KEY or LLM_API_KEY): Think bricks will fail")
	}
	eng := &engine.Engine{
		Store:   st,
		Catalog: catalog.Default(),
		Bricks:  bricks.All(),
		Brain: &llm.Brain{
			Chat:  &llm.Client{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Auth: cfg.LLMAuth, Model: cfg.LLMModel},
			Model: cfg.LLMModel,
			Store: st,
		},
		Log: log,
		// Blobs: Eurobase storage comes with Read a webpage (#26); until then
		// large fields stay inline.
	}

	host, _ := os.Hostname()
	w := &queue.Worker{
		Queue:    qeb.New(client),
		ID:       fmt.Sprintf("%s-%d", host, os.Getpid()),
		Handlers: handlers(log, eng),
		Wake:     qeb.Wakeups(ctx, cfg.EurobaseURL, cfg.EurobaseSecretKey, log),
		Log:      log,
		OnFailed: func(ctx context.Context, j queue.Job, cause error) {
			if j.Kind == engine.JobKind {
				eng.JobFailed(ctx, j, cause)
			}
		},
	}
	return w.Run(ctx)
}

// handlers maps job kinds to handlers.
func handlers(log *slog.Logger, eng *engine.Engine) map[string]queue.Handler {
	return map[string]queue.Handler{
		engine.JobKind: eng.HandleJob,
		// ping: smoke test for the queue (enqueue {"kind":"ping"} and watch it finish).
		"ping": func(_ context.Context, j queue.Job) error {
			log.Info("pong", "job", j.ID, "attempt", j.Attempts)
			return nil
		},
	}
}

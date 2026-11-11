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
	"time"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
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
		Bricks:  bricks.All(brickDeps(cfg, log, st)),
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

func brickDeps(cfg config.Config, log *slog.Logger, st store.Store) bricks.Deps {
	d := bricks.Deps{SearchCostEUR: cfg.SearchCostEUR, Store: st, Chat: &chat.Sender{}}
	if keys, err := crypto.ParseKeyring(cfg.MasterKeys, cfg.MasterKeyActive); err == nil {
		d.Keys = keys
	} else {
		log.Warn("no usable LAAMO_MASTER_KEYS: Post to chat is off", "err", err)
	}
	if cfg.BraveSearchAPIKey == "" {
		log.Warn("no BRAVE_SEARCH_API_KEY: Search the web is off")
		return d
	}
	var p search.Provider = &search.Brave{APIKey: cfg.BraveSearchAPIKey, Limit: search.NewLimiter(cfg.BraveRPS)}
	if cfg.SearchCacheDir != "" {
		p = &search.Cached{Provider: p, Dir: cfg.SearchCacheDir, TTL: 24 * time.Hour}
	}
	d.Search = p
	return d
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

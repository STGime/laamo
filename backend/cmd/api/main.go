// Command api is the Laamo HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // timezone names work in the distroless image

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/auth"
	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/demo"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/osm"
	qeb "github.com/STGime/laamo/backend/internal/queue/eurobase"
	"github.com/STGime/laamo/backend/internal/realtime"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
	"github.com/STGime/laamo/backend/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	if cfg.EurobaseSecretKey == "" {
		log.Error("config", "err", "EUROBASE_PRIVATE_KEY is required for the api")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)
	keys, err := crypto.ParseKeyring(cfg.MasterKeys, cfg.MasterKeyActive)
	if err != nil {
		log.Warn("no usable LAAMO_MASTER_KEYS: connecting chats is off", "err", err)
		keys = nil
	}
	// One realtime connection for all browsers' live updates.
	events := api.NewHub(realtime.Subscribe(ctx, cfg.EurobaseURL, cfg.EurobaseSecretKey, []string{"feed_cards", "runs", "run_steps"}, log))
	server := api.NewServer(api.Deps{
		Config: cfg,
		Store:  sebs.New(client),
		Auth:   auth.NewVerifier(cfg.EurobaseURL, cfg.EurobaseProjectID),
		Log:    log,
		Resume: func(ctx context.Context, runID string) error {
			return client.RPC(ctx, "resume_waiting_runs", map[string]any{"p_run": runID}, nil)
		},
		Events: events,
		Queue:  qeb.New(client),
		Demo:   demoService(cfg, log),
		Keys:   keys,
		Chat:   &chat.Sender{},
		Maps:   mapMaker(cfg),
	})

	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Requests share ctx, so open event streams end on shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go func() {
		log.Info("api listening", "addr", cfg.APIAddr, "env", cfg.Env, "project", cfg.EurobaseProjectID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// demoService runs sample helpers for visitors (#75). It needs the AI and
// search keys; without them the demo is off.
// mapMaker draws business card maps with Geoapify, when there's a key.
func mapMaker(cfg config.Config) api.MapMaker {
	if cfg.GeoapifyAPIKey == "" {
		return nil
	}
	return &osm.Geoapify{APIKey: cfg.GeoapifyAPIKey}
}

func demoService(cfg config.Config, log *slog.Logger) *demo.Service {
	if cfg.LLMAPIKey == "" || cfg.BraveSearchAPIKey == "" {
		log.Warn("the public demo is off: it needs an LLM key and BRAVE_SEARCH_API_KEY")
		return nil
	}
	if cfg.DemoSecret == "" && cfg.Env != "dev" {
		log.Warn("no LAAMO_DEMO_SECRET: demo challenges only work on this api instance")
	}
	var p search.Provider = &search.Brave{APIKey: cfg.BraveSearchAPIKey, Limit: search.NewLimiter(cfg.BraveRPS)}
	if cfg.SearchCacheDir != "" {
		p = &search.Cached{Provider: p, Dir: cfg.SearchCacheDir, TTL: 24 * time.Hour}
	}
	return &demo.Service{
		Catalog: catalog.Default(),
		Chat:    &llm.Client{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Auth: cfg.LLMAuth, Model: cfg.LLMModel, Log: log},
		Model:   cfg.LLMModel,
		Secret:  []byte(cfg.DemoSecret),
		Log:     log,
		Deps: func(st store.Store) bricks.Deps {
			return bricks.Deps{Search: p, SearchCostEUR: cfg.SearchCostEUR, Store: st, Pages: &web.Fetcher{}}
		},
	}
}

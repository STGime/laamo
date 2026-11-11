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
	"github.com/STGime/laamo/backend/internal/config"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/realtime"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
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
	// One realtime connection for all browsers' live updates.
	events := api.NewHub(realtime.Subscribe(ctx, cfg.EurobaseURL, cfg.EurobaseSecretKey, []string{"feed_cards", "runs"}, log))
	server := api.NewServer(api.Deps{
		Config: cfg,
		Store:  sebs.New(client),
		Auth:   auth.NewVerifier(cfg.EurobaseURL, cfg.EurobaseProjectID),
		Log:    log,
		Resume: func(ctx context.Context, runID string) error {
			return client.RPC(ctx, "resume_waiting_runs", map[string]any{"p_run": runID}, nil)
		},
		Events: events,
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

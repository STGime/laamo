// Command api is the Laamo HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
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
	server := api.NewServer(api.Deps{
		Config: cfg,
		Store:  sebs.New(eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)),
		Auth:   auth.NewVerifier(cfg.EurobaseURL, cfg.EurobaseProjectID),
		Log:    log,
	})

	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

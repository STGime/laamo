package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/STGime/laamo/backend/internal/config"
)

// BaseURL is where the API is mounted (servers.url in the spec).
const BaseURL = "/api"

// Server implements StrictServerInterface.
type Server struct {
	cfg  config.Config
	http *http.Client
	log  *slog.Logger
}

var _ StrictServerInterface = (*Server)(nil)

func NewServer(cfg config.Config, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, http: &http.Client{Timeout: 3 * time.Second}, log: log}
}

// Handler returns the API's http.Handler, with routes from the spec under
// /api and JSON errors ({error, code}) for bad requests and failures.
func (s *Server) Handler() http.Handler {
	strict := NewStrictHandlerWithOptions(s, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.log.Error("api handler failed", "path", r.URL.Path, "err", err)
			WriteError(w, http.StatusInternalServerError, "internal", "internal server error")
		},
	})
	return HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseURL: BaseURL,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
	})
}

// WriteError writes the spec's Error schema.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Error: msg, Code: code})
}

func (s *Server) GetHealth(ctx context.Context, _ GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200JSONResponse{
		Status:   "ok",
		Env:      s.cfg.Env,
		Eurobase: s.eurobaseStatus(ctx),
	}, nil
}

// eurobaseStatus reports whether the Eurobase project is configured and reachable.
func (s *Server) eurobaseStatus(ctx context.Context) HealthEurobase {
	if s.cfg.EurobaseSecretKey == "" {
		return HealthEurobaseNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.EurobaseURL, nil)
	if err != nil {
		return HealthEurobaseError
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return HealthEurobaseUnreachable
	}
	_ = resp.Body.Close()
	return HealthEurobaseReachable
}

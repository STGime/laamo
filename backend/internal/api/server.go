package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/STGime/laamo/backend/internal/queue"

	"github.com/STGime/laamo/backend/internal/auth"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/packages/openapi"
)

// BaseURL is where the API is mounted (servers.url in the spec).
const BaseURL = "/api"

// Server implements StrictServerInterface.
type Server struct {
	cfg   config.Config
	store store.Store
	auth  auth.TokenVerifier
	http  *http.Client
	log   *slog.Logger
	// resume queues a waiting run once its Feed card is decided.
	resume func(ctx context.Context, runID string) error
	events *Hub
	queue  queue.Queue
}

var _ StrictServerInterface = (*Server)(nil)

// Deps are the API's collaborators.
type Deps struct {
	Config config.Config
	Store  store.Store
	Auth   auth.TokenVerifier
	Log    *slog.Logger
	// Resume queues a waiting run after a decision (resume_waiting_runs).
	// Optional: without it the every-minute sweep picks the run up.
	Resume func(ctx context.Context, runID string) error
	// Events feeds live updates (GET /api/workspaces/{id}/events). Optional.
	Events *Hub
	// Queue takes run jobs ("Run now", the first test run).
	Queue queue.Queue
}

func NewServer(d Deps) *Server {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Server{cfg: d.Config, store: d.Store, auth: d.Auth, http: &http.Client{Timeout: 3 * time.Second}, log: d.Log,
		resume: d.Resume, events: d.Events, queue: d.Queue}
}

// maxBody caps request bodies.
const maxBody = 1 << 20

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
	routes := HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseURL: BaseURL,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
	})
	mux := http.NewServeMux()
	mux.Handle("/", routes)
	// The contract itself, for API tools (Postman, Bruno, Insomnia import it).
	mux.HandleFunc("GET "+BaseURL+"/openapi.yaml", serveSpec)
	// Live updates as Server-Sent Events (streaming, so outside the spec's
	// generated handlers; documented on listFeed).
	mux.HandleFunc("GET "+BaseURL+"/workspaces/{workspaceId}/events", s.serveEvents)

	public := func(r *http.Request) bool {
		return r.Method == http.MethodGet && (r.URL.Path == BaseURL+"/healthz" || r.URL.Path == BaseURL+"/openapi.yaml")
	}
	return http.MaxBytesHandler(auth.Middleware(s.auth, public, WriteError)(mux), maxBody)
}

// serveSpec serves the contract as written (embedded at build time).
func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openapi.Spec)
}

// WriteError writes the spec's Error schema.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Error: msg, Code: ErrorCode(code)})
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

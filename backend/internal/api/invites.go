package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/auth"
	"github.com/STGime/laamo/backend/internal/store"
)

// While Laamo is invite-only (config.InviteOnly), signing up is open at
// Eurobase but the API only serves people invited from the waiting list
// (waitlist.invited_at). Everyone else gets 403 not_invited and the app
// shows "you're not in yet".

// notInvitedFor: how long a "no" is remembered (an invite gets through
// within this time). A "yes" is remembered until the api restarts.
const notInvitedFor = time.Minute

type inviteGate struct {
	mu  sync.Mutex
	yes map[string]bool
	no  map[string]time.Time // email → until
}

func (g *inviteGate) cached(email string, now time.Time) (ok, known bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.yes[email] {
		return true, true
	}
	if until, found := g.no[email]; found && now.Before(until) {
		return false, true
	}
	return false, false
}

func (g *inviteGate) remember(email string, ok bool, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if ok {
		if g.yes == nil {
			g.yes = map[string]bool{}
		}
		g.yes[email] = true
		delete(g.no, email)
		return
	}
	if g.no == nil || len(g.no) > 10_000 {
		g.no = map[string]time.Time{}
	}
	g.no[email] = now.Add(notInvitedFor)
}

// invited reports whether the signed-in user may use the app.
func (s *Server) invited(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}
	now := time.Now()
	if ok, known := s.invites.cached(email, now); known {
		return ok, nil
	}
	e, err := s.store.WaitlistByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return false, err
	}
	ok := err == nil && e.InvitedAt != nil
	s.invites.remember(email, ok, now)
	return ok, nil
}

// inviteOnly lets signed-in requests through only for invited people.
// Public requests (no user) pass.
func (s *Server) inviteOnly(next http.Handler) http.Handler {
	if !s.cfg.InviteOnly {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, signedIn := auth.UserFrom(r.Context())
		if !signedIn {
			next.ServeHTTP(w, r)
			return
		}
		ok, err := s.invited(r.Context(), u.Email)
		if err != nil {
			s.log.Error("invite check failed", "err", err)
			WriteError(w, http.StatusServiceUnavailable, "unavailable", "can't check your invite right now")
			return
		}
		if !ok {
			WriteError(w, http.StatusForbidden, "not_invited", "Laamo is invite-only for now: join the waiting list")
			return
		}
		next.ServeHTTP(w, r)
	})
}

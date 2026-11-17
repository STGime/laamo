package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/waitlist"
)

// The waiting list (marketing site). Public: no account. Guarded by a proof
// of work per sign-up, a honeypot field, per-IP limits and a cap on how many
// confirmation emails go out per hour.

// Waitlist limits.
var (
	waitlistPerIP     = 5  // sign-ups per IP and window
	waitlistConfirmIP = 20 // confirm attempts per IP and window
	waitlistWindow    = 10 * time.Minute
	waitlistMailsHour = 300 // confirmation emails per hour, all together
	waitlistBits      = 16  // proof of work: well under a second in a phone browser
)

func (s *Server) GetWaitlistChallenge(_ context.Context, _ GetWaitlistChallengeRequestObject) (GetWaitlistChallengeResponseObject, error) {
	c := s.wlProof.New()
	return GetWaitlistChallenge200JSONResponse{Token: c.Token, Bits: c.Bits}, nil
}

func (s *Server) JoinWaitlist(ctx context.Context, req JoinWaitlistRequestObject) (JoinWaitlistResponseObject, error) {
	if s.waitlist == nil {
		return JoinWaitlist503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "the waiting list is closed"))}, nil
	}
	b := req.Body
	if !s.wlLimit.allow("join:"+clientIP(ctx), waitlistPerIP, waitlistWindow) {
		return JoinWaitlist429JSONResponse{TooManyRequestsJSONResponse(apiErr(ErrorCodeRateLimited, "too many sign-ups, try again later"))}, nil
	}
	if err := s.wlProof.Check(b.Token, b.Nonce); err != nil {
		return JoinWaitlist400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, err.Error()))}, nil
	}
	ok := JoinWaitlist202JSONResponse{Status: CheckInbox}
	// A bot filled in the hidden field: answer as usual, do nothing.
	if b.Website != nil && *b.Website != "" {
		s.log.Info("waitlist: honeypot", "ip", clientIP(ctx))
		return ok, nil
	}
	if waitlist.CleanEmail(b.Email) == "" {
		return JoinWaitlist400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, waitlist.ErrEmail.Error()))}, nil
	}
	if !s.wlLimit.allow("mail", waitlistMailsHour, time.Hour) {
		s.log.Warn("waitlist: hourly email cap reached", "cap", waitlistMailsHour)
		return JoinWaitlist503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "lots of sign-ups right now, try again in a bit"))}, nil
	}
	in := waitlist.Join{Email: b.Email, Lang: string(deref(b.Lang)), Source: deref(b.Source), Ref: deref(b.Ref)}
	if b.Interests != nil {
		for _, i := range *b.Interests {
			in.Interests = append(in.Interests, string(i))
		}
	}
	switch err := s.waitlist.Join(ctx, in); {
	case errors.Is(err, waitlist.ErrEmail):
		return JoinWaitlist400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, err.Error()))}, nil
	case err != nil:
		s.log.Error("waitlist: join", "err", err)
		return JoinWaitlist503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "that didn't work, try again in a minute"))}, nil
	}
	return ok, nil
}

func (s *Server) ConfirmWaitlist(ctx context.Context, req ConfirmWaitlistRequestObject) (ConfirmWaitlistResponseObject, error) {
	if s.waitlist == nil {
		return ConfirmWaitlist400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, waitlist.ErrToken.Error()))}, nil
	}
	if !s.wlLimit.allow("confirm:"+clientIP(ctx), waitlistConfirmIP, waitlistWindow) {
		return ConfirmWaitlist429JSONResponse{TooManyRequestsJSONResponse(apiErr(ErrorCodeRateLimited, "too many tries, try again later"))}, nil
	}
	e, err := s.waitlist.Confirm(ctx, req.Body.Token)
	if errors.Is(err, waitlist.ErrToken) {
		return ConfirmWaitlist400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, err.Error()))}, nil
	}
	if err != nil {
		return nil, err
	}
	s.log.Info("waitlist: confirmed", "locale", e.Locale)
	return ConfirmWaitlist200JSONResponse{RefCode: e.RefCode, Lang: Locale(e.Locale)}, nil
}

// limiter counts events per key in a sliding window (one api instance).
type limiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
	now    func() time.Time
}

func (l *limiter) allow(key string, n int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	if l.events == nil {
		l.events = map[string][]time.Time{}
	}
	if len(l.events) > 10000 { // forget everything rather than grow without end
		l.events = map[string][]time.Time{}
	}
	kept := l.events[key][:0]
	for _, t := range l.events[key] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= n {
		l.events[key] = kept
		return false
	}
	l.events[key] = append(kept, now)
	return true
}

package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/STGime/laamo/backend/internal/demo"
)

// The public demo (#75): a sample helper's result without an account.

func (s *Server) GetDemoChallenge(_ context.Context, _ GetDemoChallengeRequestObject) (GetDemoChallengeResponseObject, error) {
	if s.demo == nil {
		return GetDemoChallenge200JSONResponse{Token: "", Bits: 0}, nil
	}
	c := s.demo.NewChallenge()
	return GetDemoChallenge200JSONResponse{Token: c.Token, Bits: c.Bits}, nil
}

func (s *Server) RunDemo(ctx context.Context, req RunDemoRequestObject) (RunDemoResponseObject, error) {
	if s.demo == nil {
		return RunDemo503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "the demo is off"))}, nil
	}
	b := req.Body
	in := demo.Input{Template: string(b.Template), City: b.City, Token: b.Token, Nonce: b.Nonce}
	if b.Lang != nil {
		in.Lang = string(*b.Lang)
	}
	if b.Business != nil {
		in.Business = *b.Business
	}
	if b.Timezone != nil {
		in.Timezone = *b.Timezone
	}
	res, err := s.demo.Run(ctx, clientIP(ctx), in)
	switch {
	case errors.Is(err, demo.ErrInput), errors.Is(err, demo.ErrProof):
		return RunDemo400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, err.Error()))}, nil
	case errors.Is(err, demo.ErrLimit):
		return RunDemo429JSONResponse{TooManyRequestsJSONResponse(apiErr(ErrorCodeRateLimited, err.Error()))}, nil
	case errors.Is(err, demo.ErrBudget):
		return RunDemo503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, err.Error()))}, nil
	case err != nil:
		s.log.Warn("demo run failed", "err", err)
		return RunDemo503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "the demo didn't work this time"))}, nil
	}
	out := RunDemo200JSONResponse{Template: res.Template, Name: res.Name, Avatar: res.Avatar, Title: res.Title, Body: res.Body,
		Links: []DemoLink{}, Cards: []DemoCard{}, Cached: res.Cached, Seconds: new(float32(res.Seconds))}
	for _, l := range res.Links {
		out.Links = append(out.Links, DemoLink{Title: l.Title, Url: l.URL})
	}
	for _, c := range res.Cards {
		out.Cards = append(out.Cards, DemoCard{Title: c.Title, Summary: c.Summary, Url: c.URL})
	}
	return out, nil
}

type ipKey struct{}

func clientIP(ctx context.Context) string {
	ip, _ := ctx.Value(ipKey{}).(string)
	return ip
}

// withClientIP puts the caller's IP in the request context. Behind the load
// balancer (trustProxy) it is the last X-Forwarded-For hop: the one our
// proxy added, which the client can't fake.
func withClientIP(next http.Handler, trustProxy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if xff := r.Header.Get("X-Forwarded-For"); trustProxy && xff != "" {
			hops := strings.Split(xff, ",")
			if last := strings.TrimSpace(hops[len(hops)-1]); net.ParseIP(last) != nil {
				ip = last
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ipKey{}, ip)))
	})
}

package api

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"

	"github.com/STGime/laamo/backend/internal/push"
	"github.com/STGime/laamo/backend/internal/store"
)

// MaxPushSubscriptions is how many browsers one person may have
// notifications on for; the oldest gives way to a new one.
const MaxPushSubscriptions = 10

var pushOff = apiErr(ErrorCodeUnavailable, "notifications are not set up on this server")

func (s *Server) GetPushKey(_ context.Context, _ GetPushKeyRequestObject) (GetPushKeyResponseObject, error) {
	if s.push == nil {
		return GetPushKey503JSONResponse{UnavailableJSONResponse(pushOff)}, nil
	}
	return GetPushKey200JSONResponse{PublicKey: s.push.Sender.PublicKey}, nil
}

func (s *Server) SavePushSubscription(ctx context.Context, req SavePushSubscriptionRequestObject) (SavePushSubscriptionResponseObject, error) {
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	if s.push == nil {
		return SavePushSubscription503JSONResponse{UnavailableJSONResponse(pushOff)}, nil
	}
	bad := func(msg string) (SavePushSubscriptionResponseObject, error) {
		return SavePushSubscription400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, msg))}, nil
	}
	b := req.Body
	// The endpoint is where our servers will send to: only the browsers'
	// own push services, never an address of the user's choosing.
	if len(b.Endpoint) > 1000 || !s.pushAllowed(b.Endpoint) {
		return bad("endpoint is not a browser push service")
	}
	// An uncompressed P-256 point and a 16-byte secret, as browsers make them.
	if k := unb64(b.Keys.P256dh); len(k) != 65 || k[0] != 4 {
		return bad("keys.p256dh is not a browser's public key")
	}
	if len(unb64(b.Keys.Auth)) != 16 {
		return bad("keys.auth is not a browser's secret")
	}
	sub := store.PushSubscription{UserID: u.ID, Endpoint: b.Endpoint, P256dh: b.Keys.P256dh, Auth: b.Keys.Auth}
	if err := s.store.SavePushSubscription(ctx, &sub); err != nil {
		return nil, err
	}
	all, err := s.store.PushSubscriptions(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for len(all) > MaxPushSubscriptions {
		old := all[0]
		if old.ID == sub.ID {
			break
		}
		if err := s.store.DeletePushSubscription(ctx, old.ID); err != nil {
			return nil, err
		}
		all = all[1:]
	}
	return SavePushSubscription204Response{}, nil
}

func (s *Server) DeletePushSubscription(ctx context.Context, req DeletePushSubscriptionRequestObject) (DeletePushSubscriptionResponseObject, error) {
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	// Only your own: someone else's endpoint does nothing.
	if sub, ok, err := s.pushSubscription(ctx, u.ID, req.Params.Endpoint); err != nil {
		return nil, err
	} else if ok {
		if err := s.store.DeletePushSubscription(ctx, sub.ID); err != nil {
			return nil, err
		}
	}
	return DeletePushSubscription204Response{}, nil
}

func (s *Server) TestPush(ctx context.Context, req TestPushRequestObject) (TestPushResponseObject, error) {
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	if s.push == nil {
		return TestPush503JSONResponse{UnavailableJSONResponse(pushOff)}, nil
	}
	sub, ok, err := s.pushSubscription(ctx, u.ID, req.Body.Endpoint)
	if err != nil {
		return nil, err
	}
	if !ok {
		return TestPush404JSONResponse{NotFoundJSONResponse(apiErr(ErrorCodeNotFound, "notifications are not on for this browser"))}, nil
	}
	m := push.Message{Title: "Laamo", Body: "Notifications are on. This is how your helpers reach you.", URL: "/feed", Tag: "test"}
	if req.Body.Locale != nil && *req.Body.Locale == De {
		m.Body = "Mitteilungen sind an. So melden sich deine Helfer bei dir."
	}
	switch err := s.push.Sender.Send(ctx, sub, m, ""); {
	case err == nil:
		return TestPush204Response{}, nil
	case errors.Is(err, push.ErrGone):
		_ = s.store.DeletePushSubscription(ctx, sub.ID)
		return TestPush404JSONResponse{NotFoundJSONResponse(apiErr(ErrorCodeNotFound, "this browser's notifications were turned off; turn them on again"))}, nil
	default:
		s.log.Warn("test push", "err", err)
		return TestPush503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "the push service didn't take it; try again later"))}, nil
	}
}

// pushSubscription finds the user's subscription with this endpoint.
func (s *Server) pushSubscription(ctx context.Context, userID, endpoint string) (store.PushSubscription, bool, error) {
	all, err := s.store.PushSubscriptions(ctx, userID)
	if err != nil {
		return store.PushSubscription{}, false, err
	}
	i := slices.IndexFunc(all, func(p store.PushSubscription) bool { return p.Endpoint == endpoint })
	if i < 0 {
		return store.PushSubscription{}, false, nil
	}
	return all[i], true, nil
}

func (s *Server) pushAllowed(endpoint string) bool {
	if s.push.Sender.Allow != nil { // tests
		return true
	}
	return push.KnownService(endpoint)
}

// unb64 decodes base64url or base64, with or without padding (browsers
// differ), or returns nil.
func unb64(v string) []byte {
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, err := enc.DecodeString(v); err == nil {
			return b
		}
	}
	return nil
}

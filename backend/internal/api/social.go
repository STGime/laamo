package api

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/store"
)

// Posting to social media runs on Posta. A person has one Posta account,
// kept as a places row of type posta in the one workspace they use it in:
// the token encrypted like a chat webhook, and in meta whether Laamo
// created the account ("issued", managed with the partner key under the
// person's Laamo user id) or the person pasted their own key ("pasted":
// plan, billing and channels stay with Posta, and the partner API doesn't
// know it).
const (
	postaIssued = "issued"
	postaPasted = "pasted"
)

var socialOff = apiErr(ErrorCodeUnavailable, "posting to social media is not set up on this server")

// postaPlace finds the workspace's Posta account.
func (s *Server) postaPlace(ctx context.Context, workspaceID string) (store.Place, bool, error) {
	ps, err := s.store.ListPlaces(ctx, workspaceID)
	if err != nil {
		return store.Place{}, false, err
	}
	for _, p := range ps {
		if p.Type == "posta" {
			return p, true, nil
		}
	}
	return store.Place{}, false, nil
}

// postaElsewhere finds the Posta account in another workspace the user owns.
func (s *Server) postaElsewhere(ctx context.Context, userID, notIn string) (store.Place, bool, error) {
	ms, err := s.store.Memberships(ctx, userID)
	if err != nil {
		return store.Place{}, false, err
	}
	for _, m := range ms {
		if m.Role != store.RoleOwner || m.WorkspaceID == notIn {
			continue
		}
		if p, ok, err := s.postaPlace(ctx, m.WorkspaceID); err != nil || ok {
			return p, ok, err
		}
	}
	return store.Place{}, false, nil
}

// postaToken decrypts a Posta place's token.
func (s *Server) postaToken(p store.Place) (string, error) {
	if s.keys == nil || p.SecretCiphertext == nil {
		return "", errors.New("api: no keys or no token for this Posta account")
	}
	env, err := base64.StdEncoding.DecodeString(*p.SecretCiphertext)
	if err != nil {
		return "", err
	}
	token, err := s.keys.Decrypt(env, crypto.AAD("places", p.ID, "secret"))
	return string(token), err
}

// socialOf describes a workspace's Posta account. What Posta says about it
// (plan, channels) is left out when Posta can't be asked right now.
func (s *Server) socialOf(ctx context.Context, p store.Place) SocialPosting {
	out := SocialPosting{State: SocialPostingStateOn}
	source, _ := p.Meta["source"].(string)
	if source != postaPasted {
		source = postaIssued
	}
	out.Source = new(SocialPostingSource(source))
	if s.posta == nil {
		return out
	}
	if source == postaIssued {
		id, _ := p.Meta["external_user_id"].(string)
		a, err := s.posta.Partner.GetAccount(ctx, id)
		if err != nil {
			s.log.Warn("posta: read account", "place", p.ID, "err", err)
			return out
		}
		out.Channels = &a.ConnectedAccounts
		out.Plan = &SocialPlan{Type: a.Plan.Type, Active: a.Plan.IsActive, DaysLeft: a.Plan.DaysRemaining,
			MaxChannels: a.Plan.Limits.MaxSocialAccounts}
		if end := a.Plan.ValidUntil; end != nil {
			out.Plan.EndsAt = end
		} else {
			out.Plan.EndsAt = a.Plan.TrialEndsAt
		}
		return out
	}
	if token, err := s.postaToken(p); err == nil {
		if cs, err := s.posta.As(token).Channels(ctx); err == nil {
			out.Channels = new(len(cs))
		} else {
			s.log.Warn("posta: list channels", "place", p.ID, "err", err)
		}
	}
	return out
}

func (s *Server) GetSocial(ctx context.Context, req GetSocialRequestObject) (GetSocialResponseObject, error) {
	u, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetSocial404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if p, ok, err := s.postaPlace(ctx, m.WorkspaceID); err != nil {
		return nil, err
	} else if ok {
		return GetSocial200JSONResponse(s.socialOf(ctx, p)), nil
	}
	if s.posta == nil || s.keys == nil {
		return GetSocial200JSONResponse{State: SocialPostingStateUnavailable}, nil
	}
	if p, ok, err := s.postaElsewhere(ctx, u.ID, m.WorkspaceID); err != nil {
		return nil, err
	} else if ok {
		out := GetSocial200JSONResponse{State: SocialPostingStateElsewhere}
		if w, err := s.store.GetWorkspace(ctx, p.WorkspaceID); err == nil {
			out.Elsewhere = &w.Name
		}
		return out, nil
	}
	return GetSocial200JSONResponse{State: SocialPostingStateOff}, nil
}

func (s *Server) EnableSocial(ctx context.Context, req EnableSocialRequestObject) (EnableSocialResponseObject, error) {
	w, m, ok, err := s.workspaceOf(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return EnableSocial404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return EnableSocial403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can switch on social posting"))}, nil
	}
	if s.posta == nil || s.keys == nil {
		return EnableSocial503JSONResponse{UnavailableJSONResponse(socialOff)}, nil
	}
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	// On already: say so, and leave the account alone (creating it again
	// would replace its token).
	if p, ok, err := s.postaPlace(ctx, w.ID); err != nil {
		return nil, err
	} else if ok {
		return EnableSocial200JSONResponse(s.socialOf(ctx, p)), nil
	}
	if _, ok, err := s.postaElsewhere(ctx, u.ID, w.ID); err != nil {
		return nil, err
	} else if ok {
		return EnableSocial409JSONResponse(apiErr(ErrorCodePostaElsewhere, "social posting is on in another of your workspaces; switch it off there first")), nil
	}
	bad := func(code ErrorCode, msg string) (EnableSocialResponseObject, error) {
		return EnableSocial400JSONResponse{BadRequestJSONResponse(apiErr(code, msg))}, nil
	}
	down := func(what string, err error) (EnableSocialResponseObject, error) {
		s.log.Error("posta: "+what, "workspace", w.ID, "err", err)
		return EnableSocial503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, postaSaid(err)))}, nil
	}

	now := time.Now().UTC()
	var token string
	meta := map[string]any{}
	if req.Body.ApiKey != nil && strings.TrimSpace(*req.Body.ApiKey) != "" {
		// Their own Posta account: the key works if Posta lists its channels.
		token = strings.TrimSpace(*req.Body.ApiKey)
		if _, err := s.posta.As(token).Channels(ctx); errors.Is(err, posta.ErrUnauthorized) {
			return bad(ErrorCodePostaKeyInvalid, "Posta doesn't know this API key")
		} else if err != nil && !errors.Is(err, posta.ErrPlan) {
			return down("check pasted key", err)
		}
		meta["source"] = postaPasted
	} else {
		if req.Body.AcceptTerms == nil || !*req.Body.AcceptTerms {
			return bad(ErrorCodeBadRequest, "accept_terms must be true: a Posta account is created for your email address")
		}
		if u.Email == "" {
			return bad(ErrorCodeBadRequest, "your account has no email address")
		}
		a, err := s.posta.Partner.CreateAccount(ctx, posta.NewAccount{Email: u.Email, ExternalUserID: u.ID,
			DisplayName: w.Name, TermsAcceptedAt: now})
		switch {
		case errors.Is(err, posta.ErrAccountExists):
			return EnableSocial409JSONResponse(apiErr(ErrorCodePostaAccountExists, "this email already has a Posta account; send its API key instead")), nil
		case err != nil:
			return down("create account", err)
		}
		token = a.APIToken
		meta["source"], meta["external_user_id"], meta["trial"] = postaIssued, u.ID, a.Trial
		meta["terms_accepted_at"] = now.Format(time.RFC3339)
	}

	p := store.Place{ID: store.NewID(), WorkspaceID: w.ID, UserID: w.UserID, Type: "posta", Label: "Posta",
		SecretLast4: new(last4(token)), Meta: meta, VerifiedAt: &now}
	sealed, err := s.keys.Encrypt([]byte(token), crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		return nil, err
	}
	p.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := s.store.CreatePlace(ctx, &p); err != nil {
		// The Posta account exists but we hold no token: switching on
		// again makes a new token for the same account.
		return nil, err
	}
	return EnableSocial200JSONResponse(s.socialOf(ctx, p)), nil
}

func (s *Server) DisableSocial(ctx context.Context, req DisableSocialRequestObject) (DisableSocialResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DisableSocial404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return DisableSocial403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can switch off social posting"))}, nil
	}
	p, ok, err := s.postaPlace(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DisableSocial204Response{}, nil
	}
	// An account we created is deleted at Posta first: if that fails, the
	// row stays, so it can be tried again and nothing is left behind there.
	if source, _ := p.Meta["source"].(string); source != postaPasted {
		if s.posta == nil {
			return DisableSocial503JSONResponse{UnavailableJSONResponse(socialOff)}, nil
		}
		id, _ := p.Meta["external_user_id"].(string)
		if err := s.posta.Partner.DeleteAccount(ctx, id); err != nil {
			s.log.Error("posta: delete account", "place", p.ID, "err", err)
			return DisableSocial503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "Posta didn't answer; try again in a moment"))}, nil
		}
	}
	if err := s.store.DeletePlace(ctx, p.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return DisableSocial204Response{}, nil
}

// postaSaid says what went wrong at Posta, for the person at the screen:
// Posta's own status, code and message when it answered (no keys in
// there), else that it couldn't be reached.
func postaSaid(err error) string {
	var pe *posta.Error
	if errors.As(err, &pe) {
		msg := "Posta answered " + strconv.Itoa(pe.Status)
		if pe.Code != "" {
			msg += " " + pe.Code
		}
		if pe.Reason != "" {
			msg += " (" + pe.Reason + ")"
		}
		if pe.Message != "" {
			msg += ": " + cutRunes(pe.Message, 200)
		}
		return msg
	}
	if strings.Contains(err.Error(), "answer not understood") {
		return "Posta answered in a form Laamo doesn't understand"
	}
	return "Posta couldn't be reached; try again in a moment"
}

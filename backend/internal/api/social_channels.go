package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/store"
)

// Channels: the social accounts connected in the workspace's Posta
// account. For an account Laamo created they are connected and removed
// here; for a person's own Posta account only listed (Posta refuses a
// connect that comes back to laamo.app for those).

// socialConnectedPath is where the browser returns after connecting at a
// network. Its origin (SITE_URL) must be registered at Posta.
const socialConnectedPath = "/places/social/connected"

var socialNotOn = apiErr(ErrorCodeNotFound, "social posting is not on in this workspace")

// postaUser returns the workspace's Posta account and the API as its user.
func (s *Server) postaUser(ctx context.Context, workspaceID string) (store.Place, *posta.User, bool, error) {
	p, ok, err := s.postaPlace(ctx, workspaceID)
	if err != nil || !ok || s.posta == nil {
		return p, nil, false, err
	}
	token, err := s.postaToken(p)
	if err != nil {
		return p, nil, false, err
	}
	return p, s.posta.As(token), true, nil
}

func pasted(p store.Place) bool {
	source, _ := p.Meta["source"].(string)
	return source == postaPasted
}

// planCode maps Posta's "the plan doesn't allow this" to our error code.
func planCode(err error) (ErrorCode, string) {
	var pe *posta.Error
	_ = errors.As(err, &pe)
	switch {
	case pe != nil && pe.Reason == "trial_expired":
		return ErrorCodePostaTrialExpired, "the free days are over"
	case pe != nil && pe.Reason == "account_limit":
		return ErrorCodePostaChannelLimit, "the plan's limit of connected accounts is reached"
	}
	return ErrorCodePostaPlanInactive, "social posting is paused: the plan is not active"
}

func (s *Server) ListSocialChannels(ctx context.Context, req ListSocialChannelsRequestObject) (ListSocialChannelsResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListSocialChannels404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	p, u, ok, err := s.postaUser(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListSocialChannels404JSONResponse{NotFoundJSONResponse(socialNotOn)}, nil
	}
	down := func(what string, err error) (ListSocialChannelsResponseObject, error) {
		s.log.Error("posta: "+what, "workspace", m.WorkspaceID, "err", err)
		return ListSocialChannels503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, postaSaid(err)))}, nil
	}
	cs, err := u.Channels(ctx)
	if err != nil {
		return down("list channels", err)
	}
	out := ListSocialChannels200JSONResponse{Channels: make([]SocialChannel, 0, len(cs)), Platforms: []SocialPlatform{}, Manage: !pasted(p)}
	for _, c := range cs {
		name := c.DisplayName
		if name == "" {
			name = c.Username
		}
		if name == "" {
			name = c.Platform
		}
		out.Channels = append(out.Channels, SocialChannel{Id: c.ID, Platform: c.Platform, Name: name,
			Ok: c.IsActive && c.ConnectionError == ""})
	}
	if out.Manage {
		ps, err := s.posta.Platforms(ctx)
		if err != nil {
			return down("list platforms", err)
		}
		for _, pl := range ps {
			out.Platforms = append(out.Platforms, SocialPlatform{Id: pl.ID, Name: pl.Name, Form: pl.AuthType == "app_password",
				Images: &pl.SupportsImage, Video: &pl.SupportsVideo})
		}
	}
	return out, nil
}

func (s *Server) ConnectSocialChannel(ctx context.Context, req ConnectSocialChannelRequestObject) (ConnectSocialChannelResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ConnectSocialChannel404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return ConnectSocialChannel403JSONResponse(apiErr(ErrorCodeForbidden, "only the owner can connect social accounts")), nil
	}
	p, u, ok, err := s.postaUser(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ConnectSocialChannel404JSONResponse{NotFoundJSONResponse(socialNotOn)}, nil
	}
	bad := func(code ErrorCode, msg string) (ConnectSocialChannelResponseObject, error) {
		return ConnectSocialChannel400JSONResponse{BadRequestJSONResponse(apiErr(code, msg))}, nil
	}
	if pasted(p) {
		return bad(ErrorCodeBadRequest, "this is your own Posta account: add social accounts in Posta")
	}
	// Only a network Posta has switched on, by its own list.
	platforms, err := s.posta.Platforms(ctx)
	if err != nil {
		s.log.Error("posta: list platforms", "workspace", m.WorkspaceID, "err", err)
		return ConnectSocialChannel503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, postaSaid(err)))}, nil
	}
	var platform *posta.Platform
	for i := range platforms {
		if platforms[i].ID == req.Body.Platform {
			platform = &platforms[i]
		}
	}
	if platform == nil {
		return bad(ErrorCodeBadRequest, "this network can't be connected right now")
	}

	var link string
	if platform.AuthType == "app_password" {
		id, pw := "", ""
		if req.Body.Identifier != nil {
			id = strings.TrimPrefix(strings.TrimSpace(*req.Body.Identifier), "@")
		}
		if req.Body.AppPassword != nil {
			pw = strings.TrimSpace(*req.Body.AppPassword)
		}
		if id == "" || pw == "" {
			return bad(ErrorCodeBadRequest, "identifier and app_password are needed for "+platform.Name)
		}
		err = u.ConnectBluesky(ctx, id, pw)
		// The network said no to the handle or the password.
		var pe *posta.Error
		if errors.As(err, &pe) && !errors.Is(err, posta.ErrPlan) &&
			(pe.Status == http.StatusBadRequest || pe.Status == http.StatusUnauthorized) {
			return bad(ErrorCodePostaLoginFailed, platform.Name+" didn't accept the handle or the app password")
		}
	} else {
		link, err = u.StartConnect(ctx, platform.ID, strings.TrimRight(s.cfg.SiteURL, "/")+socialConnectedPath)
	}
	switch {
	case errors.Is(err, posta.ErrPlan):
		code, msg := planCode(err)
		return ConnectSocialChannel403JSONResponse(apiErr(code, msg)), nil
	case err != nil:
		s.log.Error("posta: connect channel", "workspace", m.WorkspaceID, "platform", platform.ID, "err", err)
		return ConnectSocialChannel503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, postaSaid(err)))}, nil
	}
	out := ConnectSocialChannel200JSONResponse{}
	if link != "" {
		out.AuthorizationUrl = &link
	}
	return out, nil
}

func (s *Server) DisconnectSocialChannel(ctx context.Context, req DisconnectSocialChannelRequestObject) (DisconnectSocialChannelResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DisconnectSocialChannel404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return DisconnectSocialChannel403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can disconnect social accounts"))}, nil
	}
	p, u, ok, err := s.postaUser(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DisconnectSocialChannel404JSONResponse{NotFoundJSONResponse(socialNotOn)}, nil
	}
	if pasted(p) {
		return DisconnectSocialChannel400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "this is your own Posta account: remove social accounts in Posta"))}, nil
	}
	if err := u.Disconnect(ctx, req.ChannelId); err != nil {
		s.log.Error("posta: disconnect channel", "workspace", m.WorkspaceID, "err", err)
		return DisconnectSocialChannel503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, postaSaid(err)))}, nil
	}
	return DisconnectSocialChannel204Response{}, nil
}

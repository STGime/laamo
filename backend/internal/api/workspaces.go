package api

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/auth"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

const maxProfileValue = 500

func apiErr(code ErrorCode, msg string) Error { return Error{Code: code, Error: msg} }

// user returns the signed-in user; the auth middleware guarantees one.
func user(ctx context.Context) (auth.User, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok {
		return auth.User{}, errors.New("api: no user in context (auth middleware missing)")
	}
	return u, nil
}

func (s *Server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	members, err := s.store.Memberships(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.WorkspaceID
	}
	workspaces, err := s.store.GetWorkspaces(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.Workspace{}
	for _, w := range workspaces {
		byID[w.ID] = w
	}
	out := GetMe200JSONResponse{User: User{Id: uuid(u.ID), Email: u.Email}, Workspaces: []WorkspaceSummary{}}
	for _, m := range members {
		if w, ok := byID[m.WorkspaceID]; ok {
			out.Workspaces = append(out.Workspaces, summary(w, m.Role))
		}
	}
	return out, nil
}

func (s *Server) CreateWorkspace(ctx context.Context, req CreateWorkspaceRequestObject) (CreateWorkspaceResponseObject, error) {
	u, err := user(ctx)
	if err != nil {
		return nil, err
	}
	nw := store.NewWorkspace{OwnerID: u.ID, Name: req.Body.Name}
	if req.Body.Timezone != nil {
		nw.Timezone = *req.Body.Timezone
	}
	if req.Body.Locale != nil {
		nw.Locale = string(*req.Body.Locale)
	}
	bad := func(msg string) (CreateWorkspaceResponseObject, error) {
		return CreateWorkspace400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, msg))}, nil
	}
	// About me facts and needs from onboarding are checked before anything
	// is created, then set right after.
	var p store.WorkspacePatch
	if req.Body.Profile != nil {
		merged, msg := mergeProfile(nil, *req.Body.Profile)
		if msg != "" {
			return bad(msg)
		}
		p.Profile = merged
	}
	if req.Body.Needs != nil {
		needs, msg := checkNeeds(*req.Body.Needs)
		if msg != "" {
			return bad(msg)
		}
		p.Needs = &needs
	}
	w, err := s.store.CreateWorkspace(ctx, nw)
	if errors.Is(err, store.ErrInvalid) {
		return bad(invalidMsg(err))
	}
	if err != nil {
		return nil, err
	}
	if p.Profile != nil || p.Needs != nil {
		if w, err = s.store.UpdateWorkspace(ctx, w.ID, p); err != nil {
			return nil, err
		}
	}
	s.log.Info("workspace created", "workspace", w.ID, "user", u.ID)
	return CreateWorkspace201JSONResponse(full(w, store.RoleOwner)), nil
}

// member returns the user's membership, or ok=false when the workspace
// doesn't exist or isn't theirs (both are a 404: no probing for ids).
func (s *Server) member(ctx context.Context, workspaceID string) (auth.User, store.Member, bool, error) {
	u, err := user(ctx)
	if err != nil {
		return u, store.Member{}, false, err
	}
	m, err := s.store.Membership(ctx, workspaceID, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		return u, m, false, nil
	}
	return u, m, err == nil, err
}

var notFound = apiErr(ErrorCodeNotFound, "workspace not found")

func (s *Server) GetWorkspace(ctx context.Context, req GetWorkspaceRequestObject) (GetWorkspaceResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetWorkspace404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	w, err := s.store.GetWorkspace(ctx, m.WorkspaceID)
	if errors.Is(err, store.ErrNotFound) {
		return GetWorkspace404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetWorkspace200JSONResponse(full(w, m.Role)), nil
}

func (s *Server) UpdateWorkspace(ctx context.Context, req UpdateWorkspaceRequestObject) (UpdateWorkspaceResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return UpdateWorkspace404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return UpdateWorkspace403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can change the workspace"))}, nil
	}
	bad := func(msg string) (UpdateWorkspaceResponseObject, error) {
		return UpdateWorkspace400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, msg))}, nil
	}

	b := req.Body
	var p store.WorkspacePatch
	if b.Name != nil {
		name := strings.TrimSpace(*b.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 80 {
			return bad("name must be 1 to 80 characters")
		}
		p.Name = &name
	}
	if b.Timezone != nil {
		if !validTimezone(*b.Timezone) {
			return bad("unknown timezone " + *b.Timezone)
		}
		p.Timezone = b.Timezone
	}
	if b.Locale != nil {
		loc := string(*b.Locale)
		if loc != "en" && loc != "de" {
			return bad("locale must be en or de")
		}
		p.Locale = &loc
	}
	if b.TimeFormat != nil {
		f := string(*b.TimeFormat)
		if f != "24h" && f != "12h" {
			return bad("time format must be 24h or 12h")
		}
		p.TimeFormat = &f
	}
	if b.Profile != nil {
		w, err := s.store.GetWorkspace(ctx, m.WorkspaceID)
		if err != nil {
			return nil, err
		}
		merged, msg := mergeProfile(w.Profile, *b.Profile)
		if msg != "" {
			return bad(msg)
		}
		p.Profile = merged
	}
	if b.Needs != nil {
		needs, msg := checkNeeds(*b.Needs)
		if msg != "" {
			return bad(msg)
		}
		p.Needs = &needs
	}

	w, err := s.store.UpdateWorkspace(ctx, m.WorkspaceID, p)
	switch {
	case errors.Is(err, store.ErrInvalid):
		return bad(invalidMsg(err))
	case errors.Is(err, store.ErrNotFound):
		return UpdateWorkspace404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	case err != nil:
		return nil, err
	}
	return UpdateWorkspace200JSONResponse(full(w, m.Role)), nil
}

// mergeProfile applies About me changes: known keys only, an empty value
// forgets the fact ("Forget" in S14). The result always has at least {}.
func mergeProfile(cur map[string]any, changes map[string]string) (map[string]any, string) {
	cat := catalog.Default()
	out := maps.Clone(cur)
	if out == nil {
		out = map[string]any{}
	}
	for _, k := range slices.Sorted(maps.Keys(changes)) {
		v := strings.TrimSpace(changes[k])
		switch {
		case !cat.HasProfile(k):
			return nil, fmt.Sprintf("unknown About me fact %q", k)
		case utf8.RuneCountInString(v) > maxProfileValue:
			return nil, fmt.Sprintf("%s is longer than %d characters", k, maxProfileValue)
		case v == "":
			delete(out, k)
		default:
			out[k] = v
		}
	}
	return out, ""
}

func validTimezone(tz string) bool {
	if tz == "" || tz == "Local" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// invalidMsg strips the "store: invalid: " prefix for the user.
func invalidMsg(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, store.ErrInvalid.Error()+": "); i >= 0 {
		return msg[i+len(store.ErrInvalid.Error())+2:]
	}
	return msg
}

func uuid(s string) openapi_types.UUID {
	var u openapi_types.UUID
	_ = u.UnmarshalText([]byte(s))
	return u
}

func summary(w store.Workspace, role string) WorkspaceSummary {
	out := WorkspaceSummary{Id: uuid(w.ID), Name: w.Name, Role: Role(role), Timezone: w.Timezone,
		Locale: Locale(w.Locale), TimeFormat: TimeFormat24h, Plan: Plan(w.Plan)}
	if w.TimeFormat == "12h" {
		out.TimeFormat = TimeFormat12h
	}
	return out
}

func full(w store.Workspace, role string) Workspace {
	s := summary(w, role)
	profile := Profile{}
	for k, v := range w.Profile {
		profile[k] = fmt.Sprint(v)
	}
	needs := Needs{}
	for _, n := range w.Needs {
		needs = append(needs, Need(n))
	}
	return Workspace{Id: s.Id, Name: s.Name, Role: s.Role, Timezone: s.Timezone, Locale: s.Locale, TimeFormat: s.TimeFormat, Plan: s.Plan,
		Profile: profile, Needs: &needs, CreatedAt: w.CreatedAt}
}

// checkNeeds validates onboarding needs: known, unique, at most 3.
func checkNeeds(in Needs) ([]string, string) {
	out := []string{}
	for _, n := range in {
		switch {
		case !n.Valid():
			return nil, fmt.Sprintf("unknown need %q", n)
		case slices.Contains(out, string(n)):
			return nil, fmt.Sprintf("need %q is listed twice", n)
		}
		out = append(out, string(n))
	}
	if len(out) > 3 {
		return nil, "pick at most 3 needs"
	}
	return out, ""
}

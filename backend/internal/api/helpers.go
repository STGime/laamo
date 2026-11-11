package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/schedule"
	"github.com/STGime/laamo/backend/internal/shelf"
	"github.com/STGime/laamo/backend/internal/store"
)

func (s *Server) workspaceOf(ctx context.Context, workspaceID string) (store.Workspace, store.Member, bool, error) {
	_, m, ok, err := s.member(ctx, workspaceID)
	if err != nil || !ok {
		return store.Workspace{}, m, false, err
	}
	w, err := s.store.GetWorkspace(ctx, m.WorkspaceID)
	if errors.Is(err, store.ErrNotFound) {
		return w, m, false, nil
	}
	return w, m, err == nil, err
}

func (s *Server) ListTemplates(ctx context.Context, req ListTemplatesRequestObject) (ListTemplatesResponseObject, error) {
	w, _, ok, err := s.workspaceOf(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListTemplates404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	templates, err := catalog.Templates()
	if err != nil {
		return nil, err
	}
	cat := catalog.Default()
	views := make([]shelf.View, 0, len(templates))
	for _, t := range templates {
		views = append(views, shelf.Preview(cat, t, w))
	}
	shelf.Sort(views)
	out := make([]ShelfTemplate, 0, len(views))
	for _, v := range views {
		st := ShelfTemplate{Slug: v.Template.Slug, Name: v.Name, Promise: v.Promise, Category: Need(v.Template.Category),
			Avatar: v.Template.Avatar, Brain: v.Brain, Paid: v.Paid, Match: v.Match, Places: v.Places,
			Bricks: []BrickPreview{}, Questions: []SetupQuestion{}}
		if st.Places == nil {
			st.Places = []string{}
		}
		for _, b := range v.Bricks {
			st.Bricks = append(st.Bricks, BrickPreview{Id: b.ID, Type: b.Type, Group: BrickPreviewGroup(b.Group), Sentence: b.Sentence})
		}
		for _, q := range v.Questions {
			sq := SetupQuestion{Key: q.Key, Kind: SetupQuestionKind(q.Kind), Profile: q.Profile, Text: q.Text}
			if q.Placeholder != "" {
				sq.Placeholder = &q.Placeholder
			}
			if len(q.Fills) > 0 {
				sq.Fills = &q.Fills
			}
			st.Questions = append(st.Questions, sq)
		}
		out = append(out, st)
	}
	return ListTemplates200JSONResponse{Templates: out}, nil
}

func helperOut(h store.Helper) Helper {
	return Helper{Id: uuid(h.ID), Name: h.Name, Avatar: h.Avatar, Status: HelperStatus(h.Status), CreatedAt: h.CreatedAt}
}

func (s *Server) ListHelpers(ctx context.Context, req ListHelpersRequestObject) (ListHelpersResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListHelpers404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	hs, err := s.store.ListHelpers(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := ListHelpers200JSONResponse{Helpers: make([]Helper, 0, len(hs))}
	for _, h := range hs {
		out.Helpers = append(out.Helpers, helperOut(h))
	}
	return out, nil
}

func (s *Server) CreateHelper(ctx context.Context, req CreateHelperRequestObject) (CreateHelperResponseObject, error) {
	w, _, ok, err := s.workspaceOf(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return CreateHelper404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	bad := func(msg string) (CreateHelperResponseObject, error) {
		return CreateHelper400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, msg))}, nil
	}
	t, found := shelf.Find(req.Body.Template)
	if !found {
		return bad("unknown template " + req.Body.Template)
	}
	cat := catalog.Default()
	stack, profile, err := shelf.Install(cat, t, w, req.Body.Answers, func(id string) (store.Place, error) {
		return s.store.GetPlace(ctx, id)
	})
	if errors.Is(err, shelf.ErrAnswer) {
		return bad(err.Error())
	}
	if err != nil {
		return nil, err
	}

	// Ask once: new About me facts are remembered for every helper.
	if len(profile) > 0 {
		changes := map[string]string{}
		for k, v := range profile {
			changes[k], _ = v.(string)
		}
		merged, msg := mergeProfile(w.Profile, changes)
		if msg != "" {
			return bad(msg)
		}
		if w, err = s.store.UpdateWorkspace(ctx, w.ID, store.WorkspacePatch{Profile: merged}); err != nil {
			return nil, err
		}
	}

	raw, _ := json.Marshal(stack)
	u, _ := user(ctx)
	h := store.Helper{WorkspaceID: w.ID, UserID: w.UserID, Name: t.Name.In(w.Locale), Status: "active", CreatedVia: "shelf"}
	if t.Avatar != "" {
		h.Avatar = new(t.Avatar)
	}
	v := store.HelperVersion{Stack: raw, CreatedBy: &u.ID}
	if err := s.store.CreateHelper(ctx, &h, &v); err != nil {
		return nil, err
	}
	if err := schedule.Sync(ctx, s.store, cat, h, w, stack, time.Now()); err != nil {
		s.log.Warn("helper schedule", "helper", h.ID, "err", err)
	}
	s.log.Info("helper created", "helper", h.ID, "template", t.Slug, "workspace", w.ID)

	out := CreateHelper201JSONResponse{Helper: helperOut(h)}
	if req.Body.RunTest != nil && *req.Body.RunTest {
		runID, err := s.startRun(ctx, h.ID, "test", nil)
		if err != nil {
			s.log.Warn("first test run", "helper", h.ID, "err", err)
		} else {
			out.RunId = new(uuid(runID))
		}
	}
	return out, nil
}

func (s *Server) StartRun(ctx context.Context, req StartRunRequestObject) (StartRunResponseObject, error) {
	h, err := s.store.GetHelper(ctx, req.HelperId.String())
	if errors.Is(err, store.ErrNotFound) {
		return StartRun404JSONResponse{NotFoundJSONResponse(apiErr(ErrorCodeNotFound, "helper not found"))}, nil
	}
	if err != nil {
		return nil, err
	}
	if _, _, ok, err := s.member(ctx, h.WorkspaceID); err != nil {
		return nil, err
	} else if !ok {
		return StartRun404JSONResponse{NotFoundJSONResponse(apiErr(ErrorCodeNotFound, "helper not found"))}, nil
	}
	trigger := "manual"
	data := map[string]any{}
	if req.Body != nil {
		if req.Body.Test != nil && *req.Body.Test {
			trigger = "test"
		}
		if req.Body.Answer != nil && *req.Body.Answer != "" {
			data["answer"] = *req.Body.Answer
		}
	}
	runID, err := s.startRun(ctx, h.ID, trigger, data)
	if errors.Is(err, engine.ErrNotRunnable) {
		return StartRun409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "this helper can't run yet"))}, nil
	}
	if err != nil {
		return nil, err
	}
	return StartRun202JSONResponse{RunId: uuid(runID)}, nil
}

// startRun creates the run and queues it for the worker.
func (s *Server) startRun(ctx context.Context, helperID, trigger string, data map[string]any) (string, error) {
	if s.queue == nil {
		return "", errors.New("no job queue configured")
	}
	eng := &engine.Engine{Store: s.store, Catalog: catalog.Default(), Log: s.log}
	r, err := eng.StartRun(ctx, engine.StartRequest{HelperID: helperID, TriggerType: trigger, TriggerData: data})
	if err != nil {
		return "", err
	}
	if _, err := s.queue.Enqueue(ctx, queue.NewJob{WorkspaceID: r.WorkspaceID, Kind: engine.JobKind,
		Payload: map[string]string{"run_id": r.ID}}); err != nil {
		return r.ID, err
	}
	s.log.Info("run queued", "run", r.ID, "helper", helperID, "trigger", trigger)
	return r.ID, nil
}

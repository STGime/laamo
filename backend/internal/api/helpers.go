package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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

// activeRun is the helper's latest run if it's still going (queued,
// running or waiting for the user), with how far it got.
func (s *Server) activeRun(ctx context.Context, h store.Helper) (*RunProgress, *store.Run) {
	runs, err := s.store.ListRuns(ctx, store.RunFilter{HelperID: h.ID, Limit: 1})
	if err != nil || len(runs) == 0 {
		return nil, nil
	}
	r := runs[0]
	if r.Status != store.RunQueued && r.Status != store.RunRunning && r.Status != store.RunWaiting {
		return nil, &r
	}
	total := 0
	if v, err := s.store.GetHelperVersion(ctx, r.VersionID); err == nil {
		var stack catalog.Stack
		if json.Unmarshal(v.Stack, &stack) == nil {
			total = len(stack.Bricks)
		}
	}
	return &RunProgress{Id: uuid(r.ID), Status: RunProgressStatus(r.Status), Step: r.NextBrickIndex, Steps: total, Test: r.TestMode}, &r
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
		ho := helperOut(h)
		ho.Run, _ = s.activeRun(ctx, h)
		out.Helpers = append(out.Helpers, ho)
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
	if req.Body.Template == BlankTemplate {
		return s.createBlank(ctx, w)
	}
	t, found := shelf.Find(req.Body.Template)
	if !found {
		return bad("unknown template " + req.Body.Template)
	}
	cat := catalog.Default()
	var just []string
	if req.Body.JustThisHelper != nil {
		just = *req.Body.JustThisHelper
	}
	stack, profile, err := shelf.Install(cat, t, w, req.Body.Answers, just, func(id string) (store.Place, error) {
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
	existing, err := s.store.ListHelpers(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	h := store.Helper{WorkspaceID: w.ID, UserID: w.UserID, Name: uniqueName(t.Name.In(w.Locale), existing), Status: "active", CreatedVia: "shelf"}
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
		runID, err := s.startRun(ctx, h.ID, "", "test", nil)
		if err != nil {
			s.log.Warn("first test run", "helper", h.ID, "err", err)
		} else {
			out.RunId = new(uuid(runID))
		}
	}
	return out, nil
}

// BlankTemplate starts a helper from scratch ("Build your own"): just a
// When I tap brick, then the builder.
const BlankTemplate = "blank"

func (s *Server) createBlank(ctx context.Context, w store.Workspace) (CreateHelperResponseObject, error) {
	cat := catalog.Default()
	tap, _ := cat.Brick("when.tap")
	stack := catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{{ID: "b1", Type: "when.tap", Config: tap.Config(nil)}}}
	raw, _ := json.Marshal(stack)
	existing, err := s.store.ListHelpers(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	name := "My helper"
	if w.Locale == "de" {
		name = "Mein Helfer"
	}
	u, _ := user(ctx)
	h := store.Helper{WorkspaceID: w.ID, UserID: w.UserID, Name: uniqueName(name, existing), Status: "active", CreatedVia: "scratch"}
	v := store.HelperVersion{Stack: raw, CreatedBy: &u.ID}
	if err := s.store.CreateHelper(ctx, &h, &v); err != nil {
		return nil, err
	}
	s.log.Info("helper created", "helper", h.ID, "template", BlankTemplate, "workspace", w.ID)
	return CreateHelper201JSONResponse{Helper: helperOut(h)}, nil
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
	if run, _ := s.activeRun(ctx, h); run != nil && run.Status != RunProgressStatusWaiting {
		return StartRun409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "this helper is already running"))}, nil
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
	runID, err := s.startRun(ctx, h.ID, "", trigger, data)
	if errors.Is(err, engine.ErrNotRunnable) {
		return StartRun409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "this helper can't run yet"))}, nil
	}
	if err != nil {
		return nil, err
	}
	return StartRun202JSONResponse{RunId: uuid(runID)}, nil
}

// startRun creates the run and queues it for the worker.
// versionID "" runs the current version.
func (s *Server) startRun(ctx context.Context, helperID, versionID, trigger string, data map[string]any) (string, error) {
	if s.queue == nil {
		return "", errors.New("no job queue configured")
	}
	eng := &engine.Engine{Store: s.store, Catalog: catalog.Default(), Log: s.log}
	r, err := eng.StartRun(ctx, engine.StartRequest{HelperID: helperID, VersionID: versionID, TriggerType: trigger, TriggerData: data})
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

// helper loads a helper the signed-in user may see (members of its
// workspace); ok=false is a 404 either way.
func (s *Server) helper(ctx context.Context, id string) (store.Helper, bool, error) {
	h, err := s.store.GetHelper(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return h, false, nil
	}
	if err != nil {
		return h, false, err
	}
	_, _, ok, err := s.member(ctx, h.WorkspaceID)
	return h, ok, err
}

var helperNotFound = apiErr(ErrorCodeNotFound, "helper not found")

func (s *Server) nextRun(ctx context.Context, h store.Helper) *time.Time {
	if h.Status != "active" {
		return nil
	}
	trs, err := s.store.HelperTriggers(ctx, h.ID)
	if err != nil {
		return nil
	}
	var next *time.Time
	for _, t := range trs {
		if t.NextRunAt != nil && (next == nil || t.NextRunAt.Before(*next)) {
			next = t.NextRunAt
		}
	}
	return next
}

func (s *Server) GetHelper(ctx context.Context, req GetHelperRequestObject) (GetHelperResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetHelper404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	w, err := s.store.GetWorkspace(ctx, h.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := HelperDetail{Helper: helperOut(h), Bricks: []BrickPreview{}, Runs: []RunSummary{}}
	out.Helper.NextRunAt = s.nextRun(ctx, h)
	var latest *store.Run
	out.Helper.Run, latest = s.activeRun(ctx, h)
	if latest != nil {
		steps, err := s.store.ListSteps(ctx, latest.ID)
		if err != nil {
			return nil, err
		}
		// One entry per brick: its latest attempt.
		byBrick := map[string]StepProgress{}
		var order []string
		for _, st := range steps {
			if _, seen := byBrick[st.BrickID]; !seen {
				order = append(order, st.BrickID)
			}
			byBrick[st.BrickID] = StepProgress{BrickId: st.BrickID, Status: StepProgressStatus(st.Status), CardsIn: st.CardsIn, CardsOut: st.CardsOut}
		}
		sp := make([]StepProgress, 0, len(order))
		for _, id := range order {
			sp = append(sp, byBrick[id])
		}
		out.Steps = &sp
	}
	if h.CurrentVersionID != nil {
		v, err := s.store.GetHelperVersion(ctx, *h.CurrentVersionID)
		if err != nil {
			return nil, err
		}
		var stack catalog.Stack
		if json.Unmarshal(v.Stack, &stack) == nil {
			// Post to chat names its chat, and Save to a list its list, not their ids.
			names := map[string]string{}
			if ps, err := s.store.ListPlaces(ctx, w.ID); err == nil {
				for _, p := range ps {
					names[p.ID] = p.Label
				}
			}
			if ls, err := s.store.ListLists(ctx, w.ID); err == nil {
				for _, l := range ls {
					names[l.ID] = l.Name
				}
			}
			for i, b := range shelf.Describe(catalog.Default(), stack, w) {
				for _, key := range []string{"place", "list"} {
					if id, _ := stack.Bricks[i].Config[key].(string); names[id] != "" {
						b.Sentence = strings.ReplaceAll(b.Sentence, id, names[id])
					}
				}
				out.Bricks = append(out.Bricks, BrickPreview{Id: b.ID, Type: b.Type, Group: BrickPreviewGroup(b.Group), Sentence: b.Sentence})
			}
		}
	}
	// Shown whenever there's something to forget, even if the stack no
	// longer learns.
	if out.Learned, err = s.store.CountPickExamples(ctx, h.ID); err != nil {
		return nil, err
	}
	runs, err := s.store.ListRuns(ctx, store.RunFilter{HelperID: h.ID, Limit: 10})
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		rs := RunSummary{Id: uuid(r.ID), Status: RunSummaryStatus(r.Status), TriggerType: r.TriggerType,
			Test: r.TestMode, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, CostEur: float32(r.CostEUR)}
		if r.Status == store.RunDone {
			if rs.CardsOut, err = s.cardsOut(ctx, r.ID); err != nil {
				return nil, err
			}
		}
		out.Runs = append(out.Runs, rs)
	}
	return GetHelper200JSONResponse(out), nil
}

// cardsOut is what a finished run ended with: its last done step's cards.
func (s *Server) cardsOut(ctx context.Context, runID string) (*int, error) {
	steps, err := s.store.ListSteps(ctx, runID)
	if err != nil {
		return nil, err
	}
	for _, st := range slices.Backward(steps) {
		if st.Status == "done" {
			return &st.CardsOut, nil
		}
	}
	return nil, nil
}

func (s *Server) UpdateHelper(ctx context.Context, req UpdateHelperRequestObject) (UpdateHelperResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return UpdateHelper404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	var p store.HelperPatch
	if req.Body.Name != nil {
		name := strings.TrimSpace(*req.Body.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 80 {
			return UpdateHelper400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "name must be 1 to 80 characters"))}, nil
		}
		p.Name = &name
	}
	if req.Body.Status != nil {
		st := string(*req.Body.Status)
		if h.Status == "draft" && st == "active" && h.CurrentVersionID == nil {
			return UpdateHelper400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "this helper has no bricks yet"))}, nil
		}
		p.Status = &st
	}
	h, err = s.store.UpdateHelper(ctx, h.ID, p)
	if err != nil {
		return nil, err
	}
	out := helperOut(h)
	out.NextRunAt = s.nextRun(ctx, h)
	return UpdateHelper200JSONResponse(out), nil
}

func (s *Server) DeleteHelper(ctx context.Context, req DeleteHelperRequestObject) (DeleteHelperResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteHelper404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	if err := s.store.DeleteHelper(ctx, h.ID); err != nil {
		return nil, err
	}
	s.log.Info("helper deleted", "helper", h.ID, "workspace", h.WorkspaceID)
	return DeleteHelper204Response{}, nil
}

// ForgetLearned deletes what a helper learned from Let me pick swipes (#76).
func (s *Server) ForgetLearned(ctx context.Context, req ForgetLearnedRequestObject) (ForgetLearnedResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ForgetLearned404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	if err := s.store.ForgetPickExamples(ctx, h.ID); err != nil {
		return nil, err
	}
	s.log.Info("helper forgot its swipes", "helper", h.ID)
	return ForgetLearned204Response{}, nil
}

// uniqueName adds " (2)", " (3)", … when the workspace already has a helper
// with this name.
func uniqueName(name string, existing []store.Helper) string {
	taken := map[string]bool{}
	for _, h := range existing {
		taken[h.Name] = true
	}
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s (%d)", name, i); !taken[n] {
			return n
		}
	}
}

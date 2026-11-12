package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/schedule"
	"github.com/STGime/laamo/backend/internal/store"
)

// The builder (#39): load a helper's stack, save an edit as a new version.

func (s *Server) GetHelperStack(ctx context.Context, req GetHelperStackRequestObject) (GetHelperStackResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok || h.CurrentVersionID == nil {
		return GetHelperStack404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	v, err := s.store.GetHelperVersion(ctx, *h.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	out, err := s.stackOut(ctx, h, v)
	if err != nil {
		return nil, err
	}
	return GetHelperStack200JSONResponse(out), nil
}

func (s *Server) SaveHelperStack(ctx context.Context, req SaveHelperStackRequestObject) (SaveHelperStackResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok || h.CurrentVersionID == nil {
		return SaveHelperStack404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	cur, err := s.store.GetHelperVersion(ctx, *h.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	stale := SaveHelperStack409JSONResponse(apiErr(ErrorCodeConflict, "this helper was changed somewhere else; reload to see it"))
	if req.Body.Version != cur.Version {
		return stale, nil
	}
	w, err := s.store.GetWorkspace(ctx, h.WorkspaceID)
	if err != nil {
		return nil, err
	}
	stack, problems, ok := s.checkStack(ctx, req.Body.Stack, w.ID)
	if !ok {
		return SaveHelperStack400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "that isn't a stack"))}, nil
	}
	if problems != nil {
		return SaveHelperStack422JSONResponse(*problems), nil
	}
	v, err := s.addVersion(ctx, h, stack, true)
	if errors.Is(err, store.ErrConflict) {
		return stale, nil
	} else if err != nil {
		return nil, err
	}
	h.CurrentVersionID = &v.ID
	if err := schedule.Sync(ctx, s.store, catalog.Default(), h, w, stack, time.Now()); err != nil {
		s.log.Warn("helper schedule", "helper", h.ID, "err", err)
	}
	s.log.Info("helper saved", "helper", h.ID, "version", v.Version)
	out, err := s.stackOut(ctx, h, v)
	if err != nil {
		return nil, err
	}
	return SaveHelperStack200JSONResponse(out), nil
}

// TestHelperStack (#42) runs an edit once in test mode (max 3 cards, Do
// bricks only reach the user) as a version that isn't made current: the
// helper keeps running what was saved.
func (s *Server) TestHelperStack(ctx context.Context, req TestHelperStackRequestObject) (TestHelperStackResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return TestHelperStack404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	if run, _ := s.activeRun(ctx, h); run != nil && run.Status != RunProgressStatusWaiting {
		return TestHelperStack409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "this helper is already running"))}, nil
	}
	stack, problems, ok := s.checkStack(ctx, req.Body.Stack, h.WorkspaceID)
	if !ok {
		return TestHelperStack400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "that isn't a stack"))}, nil
	}
	if problems != nil {
		return TestHelperStack422JSONResponse(*problems), nil
	}
	v, err := s.addVersion(ctx, h, stack, false)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if req.Body.Answer != nil && *req.Body.Answer != "" {
		data["answer"] = *req.Body.Answer
	}
	runID, err := s.startRun(ctx, h.ID, v.ID, "test", data)
	if err != nil {
		return nil, err
	}
	return TestHelperStack202JSONResponse{RunId: uuid(runID)}, nil
}

// checkStack decodes a stack from the builder and checks it like the
// engine would; ok=false when it isn't a stack at all.
func (s *Server) checkStack(ctx context.Context, in Stack, workspaceID string) (catalog.Stack, *StackProblems, bool) {
	var stack catalog.Stack
	raw, _ := json.Marshal(in)
	if err := json.Unmarshal(raw, &stack); err != nil {
		return stack, nil, false
	}
	cat := catalog.Default()
	problems := catalog.Errors(cat.Validate(stack, catalog.Options{}))
	problems = append(problems, s.foreignRefs(ctx, cat, stack, workspaceID)...)
	if len(problems) == 0 {
		return stack, nil, true
	}
	out := &StackProblems{Error: problems[0].Message, Issues: make([]StackIssue, 0, len(problems))}
	for _, p := range problems {
		out.Issues = append(out.Issues, issueOut(p))
	}
	return stack, out, true
}

func (s *Server) addVersion(ctx context.Context, h store.Helper, stack catalog.Stack, current bool) (store.HelperVersion, error) {
	raw, _ := json.Marshal(stack)
	u, _ := user(ctx)
	v := store.HelperVersion{HelperID: h.ID, Stack: raw, CreatedBy: &u.ID}
	err := s.store.AddHelperVersion(ctx, &v, current)
	return v, err
}

// foreignRefs: a Post to chat brick may only post to this workspace's
// chats (an id from elsewhere would leak results there), and Save to a list
// only save to its lists.
func (s *Server) foreignRefs(ctx context.Context, cat *catalog.Catalog, st catalog.Stack, workspaceID string) []catalog.Issue {
	var out []catalog.Issue
	for i, sb := range st.Bricks {
		b, ok := cat.Brick(sb.Type)
		if !ok {
			continue
		}
		for _, set := range b.Settings {
			id, _ := sb.Config[set.Key].(string)
			if id == "" {
				continue
			}
			switch set.Type {
			case catalog.TypePlace:
				if p, err := s.store.GetPlace(ctx, id); err == nil && p.WorkspaceID == workspaceID {
					continue
				}
				out = append(out, catalog.Issue{Brick: sb.ID, Index: i, Level: catalog.LevelError, Code: catalog.CodeBadValue,
					Setting: set.Key, Message: fmt.Sprintf("%s: pick one of your chats", b.Title.In("en"))})
			case catalog.TypeList:
				if l, err := s.store.GetList(ctx, id); err == nil && l.WorkspaceID == workspaceID {
					continue
				}
				out = append(out, catalog.Issue{Brick: sb.ID, Index: i, Level: catalog.LevelError, Code: catalog.CodeBadValue,
					Setting: set.Key, Message: fmt.Sprintf("%s: pick one of your lists", b.Title.In("en"))})
			}
		}
	}
	return out
}

func (s *Server) stackOut(ctx context.Context, h store.Helper, v store.HelperVersion) (HelperStack, error) {
	out := HelperStack{Version: v.Version, LastOut: map[string]int{}}
	if err := json.Unmarshal(v.Stack, &out.Stack); err != nil {
		return out, err
	}
	// What each brick gave the last time a run got through.
	runs, err := s.store.ListRuns(ctx, store.RunFilter{HelperID: h.ID, Limit: 10})
	if err != nil {
		return out, err
	}
	for _, r := range runs {
		if r.Status != store.RunDone && r.Status != store.RunWaiting {
			continue
		}
		steps, err := s.store.ListSteps(ctx, r.ID)
		if err != nil {
			return out, err
		}
		for _, st := range steps { // later attempts win
			if st.Status == "done" {
				out.LastOut[st.BrickID] = st.CardsOut
			}
		}
		break
	}
	return out, nil
}

func issueOut(i catalog.Issue) StackIssue {
	out := StackIssue{Index: i.Index, Level: StackIssueLevel(i.Level), Code: i.Code, Message: i.Message}
	if i.Brick != "" {
		out.Brick = &i.Brick
	}
	if i.Setting != "" {
		out.Setting = &i.Setting
	}
	if i.Field != "" {
		out.Field = &i.Field
	}
	return out
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/store"
)

var feedNotFound = apiErr(ErrorCodeNotFound, "feed card not found")

// feedCard loads a Feed card the signed-in user may see (members of its
// workspace); ok=false is a 404 either way.
func (s *Server) feedCard(ctx context.Context, id string) (store.FeedCard, string, bool, error) {
	u, err := user(ctx)
	if err != nil {
		return store.FeedCard{}, "", false, err
	}
	fc, err := s.store.GetFeedCard(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return fc, u.ID, false, nil
	}
	if err != nil {
		return fc, u.ID, false, err
	}
	if _, err := s.store.Membership(ctx, fc.WorkspaceID, u.ID); errors.Is(err, store.ErrNotFound) {
		return fc, u.ID, false, nil
	} else if err != nil {
		return fc, u.ID, false, err
	}
	return fc, u.ID, true, nil
}

func (s *Server) shownCards(ctx context.Context, fc store.FeedCard) ([]store.Card, error) {
	if fc.StepID == nil || (fc.Type != engine.FeedApproval && fc.Type != engine.FeedPick && fc.Type != "result") {
		return nil, nil
	}
	return s.store.ListCards(ctx, *fc.StepID)
}

func (s *Server) GetFeedCard(ctx context.Context, req GetFeedCardRequestObject) (GetFeedCardResponseObject, error) {
	fc, _, ok, err := s.feedCard(ctx, req.FeedCardId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetFeedCard404JSONResponse{NotFoundJSONResponse(feedNotFound)}, nil
	}
	shown, err := s.shownCards(ctx, fc)
	if err != nil {
		return nil, err
	}
	f := feedCardOut(fc, s.helperRefs(ctx, fc))
	out := FeedCardDetail{Id: f.Id, WorkspaceId: f.WorkspaceId, HelperId: f.HelperId, RunId: f.RunId, Helper: f.Helper,
		Type: FeedCardDetailType(f.Type), Title: f.Title, Body: f.Body, Payload: f.Payload,
		State: FeedCardDetailState(f.State), Decision: f.Decision, ExpiresAt: f.ExpiresAt, DecidedAt: f.DecidedAt,
		CreatedAt: f.CreatedAt, Cards: make([]Card, 0, len(shown))}
	for _, c := range shown {
		out.Cards = append(out.Cards, cardOut(c))
	}
	return GetFeedCard200JSONResponse(out), nil
}

func (s *Server) DecideFeedCard(ctx context.Context, req DecideFeedCardRequestObject) (DecideFeedCardResponseObject, error) {
	fc, userID, ok, err := s.feedCard(ctx, req.FeedCardId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DecideFeedCard404JSONResponse{NotFoundJSONResponse(feedNotFound)}, nil
	}
	conflict := func(msg string) (DecideFeedCardResponseObject, error) {
		return DecideFeedCard409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, msg))}, nil
	}
	now := time.Now()
	switch {
	case fc.State != engine.StateOpen:
		return conflict("this card is already " + fc.State)
	case fc.ExpiresAt != nil && !now.Before(*fc.ExpiresAt):
		return conflict("this card has expired")
	}

	b := req.Body
	d := engine.Decision{Skip: ids(b.Skip), Picked: ids(b.Picked)}
	if b.Answer != nil {
		d.Answer = *b.Answer
	}
	if b.Edits != nil {
		d.Edits = map[string]map[string]any{}
		for id, fields := range *b.Edits {
			d.Edits[id] = map[string]any{}
			for k, v := range fields {
				d.Edits[id][k] = v
			}
		}
	}
	state := string(b.State)
	shown, err := s.shownCards(ctx, fc)
	if err != nil {
		return nil, err
	}
	if state == engine.StateSkipped { // "skip everything" needs no list
		d.Skip = nil
		for _, c := range shown {
			d.Skip = append(d.Skip, c.CardID)
		}
	}
	if err := engine.CheckDecision(fc, shown, state, d); err != nil {
		return DecideFeedCard400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, err.Error()))}, nil
	}

	decision, err := toMap(d)
	if err != nil {
		return nil, err
	}
	fc, err = s.store.UpdateFeedCard(ctx, fc.ID, store.FeedCardPatch{State: &state, Decision: decision,
		DecidedBy: &userID, DecidedAt: &now})
	if err != nil {
		return nil, err
	}
	s.log.Info("feed card decided", "feed_card", fc.ID, "state", state, "user", userID)
	// Bring the run back now; the every-minute sweep catches it otherwise.
	if fc.RunID != nil && s.resume != nil {
		if err := s.resume(ctx, *fc.RunID); err != nil {
			s.log.Warn("resume run", "run", *fc.RunID, "err", err)
		}
	}
	return DecideFeedCard200JSONResponse(feedCardOut(fc, s.helperRefs(ctx, fc))), nil
}

func ids(list *[]openapi_types.UUID) []string {
	if list == nil {
		return nil
	}
	out := make([]string, len(*list))
	for i, id := range *list {
		out[i] = id.String()
	}
	return out
}

func uuidPtr(s *string) *openapi_types.UUID {
	if s == nil {
		return nil
	}
	return new(uuid(*s))
}

func feedCardOut(fc store.FeedCard, helpers map[string]HelperRef) FeedCard {
	out := FeedCard{Id: uuid(fc.ID), WorkspaceId: uuid(fc.WorkspaceID), HelperId: uuidPtr(fc.HelperID),
		RunId: uuidPtr(fc.RunID), Type: FeedCardType(fc.Type), Title: fc.Title, Body: fc.Body, Payload: fc.Payload,
		State: FeedCardState(fc.State), ExpiresAt: fc.ExpiresAt, DecidedAt: fc.DecidedAt, CreatedAt: fc.CreatedAt}
	if out.Payload == nil {
		out.Payload = map[string]any{}
	}
	if len(fc.Decision) > 0 {
		out.Decision = &fc.Decision
	}
	if fc.HelperID != nil {
		if h, ok := helpers[*fc.HelperID]; ok {
			out.Helper = &h
		}
	}
	return out
}

// helperRefs loads the helpers of these cards (name and avatar for the
// Feed). A deleted helper is simply left out.
func (s *Server) helperRefs(ctx context.Context, cards ...store.FeedCard) map[string]HelperRef {
	out := map[string]HelperRef{}
	for _, c := range cards {
		if c.HelperID == nil {
			continue
		}
		if _, done := out[*c.HelperID]; done {
			continue
		}
		h, err := s.store.GetHelper(ctx, *c.HelperID)
		if err != nil {
			continue
		}
		out[h.ID] = HelperRef{Id: uuid(h.ID), Name: h.Name, Avatar: h.Avatar}
	}
	return out
}

var askTypes = []string{engine.FeedApproval, engine.FeedPick, engine.FeedQuestion}

func (s *Server) ListFeed(ctx context.Context, req ListFeedRequestObject) (ListFeedResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListFeed404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	limit := 30
	if req.Params.Limit != nil {
		limit = min(max(*req.Params.Limit, 1), 100)
	}
	f := store.FeedFilter{WorkspaceID: m.WorkspaceID, Before: req.Params.Before, Limit: limit + 1}
	filter := All
	if req.Params.Filter != nil {
		filter = *req.Params.Filter
	}
	switch filter {
	case NeedsMe:
		f.Types, f.States = askTypes, []string{engine.StateOpen}
	case Results:
		f.Types, f.NotStates = []string{"result", "problem"}, []string{engine.StateDismiss}
	default:
		f.NotStates = []string{engine.StateDismiss}
	}
	cards, err := s.store.ListFeedCards(ctx, f)
	if err != nil {
		return nil, err
	}
	page := FeedPage{Cards: make([]FeedCard, 0, len(cards))}
	if len(cards) > limit {
		cards = cards[:limit]
		page.NextBefore = &cards[limit-1].CreatedAt
	}
	helpers := s.helperRefs(ctx, cards...)
	for _, c := range cards {
		page.Cards = append(page.Cards, feedCardOut(c, helpers))
	}
	open, err := s.store.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: m.WorkspaceID, Types: askTypes,
		States: []string{engine.StateOpen}, Limit: 100})
	if err != nil {
		return nil, err
	}
	page.NeedsMe = len(open)
	page.Brain = BrainMeter{Mode: Included, AllowanceCredits: 500, CreditsLeft: 500}
	if b, err := s.store.GetBrain(ctx, m.WorkspaceID); err == nil {
		page.Brain = BrainMeter{Mode: BrainMeterMode(b.Mode), AllowanceCredits: float32(b.AllowanceCredits),
			CreditsLeft: float32(max(0, b.CreditsLeft(time.Now())))}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return ListFeed200JSONResponse(page), nil
}

func (s *Server) DismissFeedCard(ctx context.Context, req DismissFeedCardRequestObject) (DismissFeedCardResponseObject, error) {
	fc, _, ok, err := s.feedCard(ctx, req.FeedCardId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DismissFeedCard404JSONResponse{NotFoundJSONResponse(feedNotFound)}, nil
	}
	if fc.State == engine.StateOpen && fc.Type != "result" && fc.Type != "problem" {
		return DismissFeedCard409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "answer this card first"))}, nil
	}
	if fc.State != engine.StateDismiss {
		if fc, err = s.store.UpdateFeedCard(ctx, fc.ID, store.FeedCardPatch{State: new(engine.StateDismiss)}); err != nil {
			return nil, err
		}
	}
	return DismissFeedCard200JSONResponse(feedCardOut(fc, s.helperRefs(ctx, fc))), nil
}

func cardOut(c store.Card) Card {
	out := Card{Id: uuid(c.CardID), Kind: c.Kind, Fields: c.Data, Trust: map[string]CardTrust{},
		SourceUrl: c.SourceURL, Reason: c.Reason}
	if out.Fields == nil {
		out.Fields = map[string]any{}
	}
	for k, v := range c.Trust {
		out.Trust[k] = CardTrust(v)
	}
	if c.Score != nil {
		out.Score = new(float32(*c.Score))
	}
	return out
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(raw, &m)
}

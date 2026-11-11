package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/store"
)

// Places (#34): the chats helpers post to. A webhook URL is a secret: it is
// encrypted before it's stored, and never sent back to the app.

// ChatSender posts a message to a webhook (chat.Sender).
type ChatSender interface {
	Send(ctx context.Context, placeType, webhook string, msg chat.Message) error
}

var placeNotFound = apiErr(ErrorCodeNotFound, "chat not found")

func (s *Server) ListPlaces(ctx context.Context, req ListPlacesRequestObject) (ListPlacesResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListPlaces404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	ps, err := s.store.ListPlaces(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	use, err := s.placeUse(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := ListPlaces200JSONResponse{Places: make([]Place, 0, len(ps))}
	for _, p := range ps {
		po := placeOut(p)
		po.UsedBy = new(use[p.ID])
		out.Places = append(out.Places, po)
	}
	return out, nil
}

func (s *Server) CreatePlace(ctx context.Context, req CreatePlaceRequestObject) (CreatePlaceResponseObject, error) {
	w, m, ok, err := s.workspaceOf(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return CreatePlace404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return CreatePlace403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can connect chats"))}, nil
	}
	if s.keys == nil || s.chat == nil {
		return CreatePlace503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "chats are off: the server has no LAAMO_MASTER_KEYS"))}, nil
	}
	b := req.Body
	label := strings.TrimSpace(b.Label)
	if n := utf8.RuneCountInString(label); n < 1 || n > 60 {
		return CreatePlace400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "name must be 1 to 60 characters"))}, nil
	}
	webhook := strings.TrimSpace(b.WebhookUrl)
	typ := string(b.Type)
	if err := chat.CheckWebhook(typ, webhook); err != nil {
		return CreatePlace400JSONResponse{BadRequestJSONResponse(webhookErr(err))}, nil
	}
	// The test message first: only a chat that got it is saved.
	if err := s.chat.Send(ctx, typ, webhook, testMessage(w.Locale)); err != nil {
		s.log.Info("chat test failed", "type", typ, "err", err)
		return CreatePlace400JSONResponse{BadRequestJSONResponse(webhookErr(err))}, nil
	}
	now := time.Now().UTC()
	p := store.Place{ID: store.NewID(), WorkspaceID: w.ID, UserID: w.UserID, Type: typ, Label: label,
		SecretLast4: new(last4(webhook)), VerifiedAt: &now}
	sealed, err := s.keys.Encrypt([]byte(webhook), crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		return nil, err
	}
	p.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := s.store.CreatePlace(ctx, &p); err != nil {
		return nil, err
	}
	out := placeOut(p)
	out.UsedBy = new([]string{})
	return CreatePlace201JSONResponse(out), nil
}

func (s *Server) TestPlace(ctx context.Context, req TestPlaceRequestObject) (TestPlaceResponseObject, error) {
	p, w, ok, err := s.place(ctx, req.PlaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return TestPlace404JSONResponse{NotFoundJSONResponse(placeNotFound)}, nil
	}
	if s.keys == nil || s.chat == nil || p.SecretCiphertext == nil {
		return TestPlace503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "this chat can't be tested here"))}, nil
	}
	env, err := base64.StdEncoding.DecodeString(*p.SecretCiphertext)
	if err != nil {
		return nil, err
	}
	webhook, err := s.keys.Decrypt(env, crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		return TestPlace503JSONResponse{UnavailableJSONResponse(apiErr(ErrorCodeUnavailable, "this chat's address can't be read; connect it again"))}, nil
	}
	if err := s.chat.Send(ctx, p.Type, string(webhook), testMessage(w.Locale)); err != nil {
		return TestPlace400JSONResponse{BadRequestJSONResponse(webhookErr(err))}, nil
	}
	now := time.Now().UTC()
	if p, err = s.store.UpdatePlace(ctx, p.ID, store.PlacePatch{VerifiedAt: &now}); err != nil {
		return nil, err
	}
	use, err := s.placeUse(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := placeOut(p)
	out.UsedBy = new(use[p.ID])
	return TestPlace200JSONResponse(out), nil
}

func (s *Server) DeletePlace(ctx context.Context, req DeletePlaceRequestObject) (DeletePlaceResponseObject, error) {
	p, _, ok, err := s.place(ctx, req.PlaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeletePlace404JSONResponse{NotFoundJSONResponse(placeNotFound)}, nil
	}
	_, m, _, err := s.member(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if m.Role != store.RoleOwner {
		return DeletePlace403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can remove chats"))}, nil
	}
	if err := s.store.DeletePlace(ctx, p.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return DeletePlace204Response{}, nil
}

// place loads a place the caller may see, with its workspace.
func (s *Server) place(ctx context.Context, id string) (store.Place, store.Workspace, bool, error) {
	p, err := s.store.GetPlace(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return p, store.Workspace{}, false, nil
	}
	if err != nil {
		return p, store.Workspace{}, false, err
	}
	w, _, ok, err := s.workspaceOf(ctx, p.WorkspaceID)
	return p, w, ok, err
}

// placeUse maps place ids to the names of the helpers that post there.
func (s *Server) placeUse(ctx context.Context, workspaceID string) (map[string][]string, error) {
	hs, err := s.store.ListHelpers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	use := map[string][]string{}
	for _, h := range hs {
		if h.CurrentVersionID == nil {
			continue
		}
		v, err := s.store.GetHelperVersion(ctx, *h.CurrentVersionID)
		if err != nil {
			continue
		}
		var stack catalog.Stack
		if json.Unmarshal(v.Stack, &stack) != nil {
			continue
		}
		for _, b := range stack.Bricks {
			if id, _ := b.Config["place"].(string); id != "" && b.Type == "do.chat" {
				use[id] = append(use[id], h.Name)
				break
			}
		}
	}
	return use, nil
}

func placeOut(p store.Place) Place {
	out := Place{Id: uuid(p.ID), Type: PlaceType(p.Type), Label: p.Label, CreatedAt: p.CreatedAt, VerifiedAt: p.VerifiedAt}
	if p.SecretLast4 != nil {
		out.Last4 = p.SecretLast4
	}
	return out
}

func last4(s string) string {
	r := []rune(s)
	return string(r[max(0, len(r)-4):])
}

func testMessage(lang string) chat.Message {
	if lang == "de" {
		return chat.Message{Text: "✅ **Laamo ist verbunden.** Deine Helfer schreiben ab jetzt hier."}
	}
	return chat.Message{Text: "✅ **Laamo is connected.** Your helpers will post here from now on."}
}

// webhookErr says why a chat didn't take the test message.
func webhookErr(err error) Error {
	switch {
	case errors.Is(err, chat.ErrBadWebhook), errors.Is(err, chat.ErrUnsupported):
		return apiErr(ErrorCodeWebhookInvalid, "that isn't a webhook link for this chat")
	case errors.Is(err, chat.ErrGone):
		return apiErr(ErrorCodeWebhookGone, "the chat says this webhook doesn't exist (anymore)")
	case errors.Is(err, chat.ErrRejected):
		return apiErr(ErrorCodeWebhookRejected, "the chat didn't accept the test message")
	}
	return apiErr(ErrorCodeWebhookUnreachable, "couldn't reach the chat; try again in a moment")
}

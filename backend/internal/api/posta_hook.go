package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	guuid "github.com/google/uuid"

	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/store"
)

// Posta tells Laamo how a post did (dev guide 5.11): it calls
//
//	POST /api/posta/webhook/<place id>
//	X-Posta-Timestamp, X-Posta-Signature: hex HMAC-SHA256 of "<timestamp>.<body>"
//
// with the secret of the endpoint the Post to social brick registered for
// that account (do.Social.listen). The result becomes a Feed card, which
// also goes out as a notification.
//
// Answers: 200 taken (or nothing to do: a ping, a post Laamo didn't make,
// a result that was told before), 401 bad signature, 404 no such account,
// 503 try again (Posta retries up to five times).
const PostaHookPath = posta.HookPath

// maxPostaHook: a delivery is a few hundred bytes.
const maxPostaHook = 64 << 10

// postaResultSpace makes card ids: one card per post and result, however
// often Posta tells.
var postaResultSpace = guuid.MustParse("8a2f5d3c-1b7e-4c0a-9f44-706f73746121")

func (s *Server) servePostaHook(w http.ResponseWriter, r *http.Request) {
	answer := func(status int, outcome string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"outcome": outcome})
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxPostaHook+1))
	if err != nil || len(raw) > maxPostaHook {
		answer(http.StatusBadRequest, "unreadable")
		return
	}
	id := r.PathValue("place")
	if _, err := guuid.Parse(id); err != nil {
		answer(http.StatusNotFound, "no_such_account")
		return
	}
	place, err := s.store.GetPlace(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && place.Type != "posta") {
		answer(http.StatusNotFound, "no_such_account")
		return
	}
	if err != nil {
		s.log.Error("posta webhook", "place", id, "err", err)
		answer(http.StatusServiceUnavailable, "try_later")
		return
	}
	secret, ok := s.postaHookSecret(place)
	if !ok {
		answer(http.StatusNotFound, "no_such_account")
		return
	}
	if !posta.Signed(secret, r.Header.Get("X-Posta-Timestamp"), r.Header.Get("X-Posta-Signature"), raw) {
		answer(http.StatusUnauthorized, "bad_signature")
		return
	}
	var d posta.Delivery
	if err := json.Unmarshal(raw, &d); err != nil {
		answer(http.StatusBadRequest, "unreadable")
		return
	}
	outcome, err := s.takePostResult(r.Context(), place, d)
	if err != nil {
		s.log.Error("posta webhook", "place", id, "event", d.Event, "err", err)
		answer(http.StatusServiceUnavailable, "try_later")
		return
	}
	s.log.Info("posta webhook", "place", id, "event", d.Event, "outcome", outcome)
	answer(http.StatusOK, outcome)
}

// postaHookSecret decrypts the secret of a place's webhook.
func (s *Server) postaHookSecret(p store.Place) (string, bool) {
	hook, _ := p.Meta[posta.MetaHook].(map[string]any)
	enc, _ := hook[posta.MetaHookSecret].(string)
	if s.keys == nil || enc == "" {
		return "", false
	}
	env, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	secret, err := s.keys.Decrypt(env, crypto.AAD("places", p.ID, posta.HookField))
	return string(secret), err == nil && len(secret) > 0
}

// takePostResult turns a delivery into a Feed card and notes the result on
// the message. The same delivery twice makes one card.
func (s *Server) takePostResult(ctx context.Context, place store.Place, d posta.Delivery) (string, error) {
	var failed bool
	switch d.Event {
	case "post.published":
	case "post.failed":
		failed = true
	default:
		return "ignored", nil // a ping, or an event we didn't ask for
	}
	if d.Data.Post.ID == "" {
		return "ignored", nil
	}
	// Only posts Laamo made: someone with their own Posta account also
	// posts from there.
	m, err := s.store.GetOutboundByProviderID(ctx, place.WorkspaceID, "posta", d.Data.Post.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "not_ours", nil
	}
	if err != nil {
		return "", err
	}
	de := false
	if w, err := s.store.GetWorkspace(ctx, place.WorkspaceID); err == nil {
		de = w.Locale == "de"
	}
	title, body := postResultText(de, failed, d.Data.Results, deref(m.Body))
	card := store.FeedCard{
		ID:          guuid.NewSHA1(postaResultSpace, []byte(d.Data.Post.ID+"/"+d.Event)).String(),
		WorkspaceID: m.WorkspaceID, UserID: m.UserID, Type: "result", Title: title, Body: &body,
		Payload: map[string]any{"social_post": d.Data.Post.ID},
	}
	if failed {
		card.Type = "problem"
	}
	// The card names its helper but not the run: the run is long over, and
	// a card of a run started by hand would go out without a notification.
	if m.RunID != nil {
		if run, err := s.store.GetRun(ctx, *m.RunID); err == nil && run.HelperID != "" {
			card.HelperID = &run.HelperID
		}
	}
	if err := s.store.CreateFeedCard(ctx, &card); err != nil && !errors.Is(err, store.ErrConflict) {
		return "", err
	}
	patch := store.OutboundPatch{Status: new("sent"), SentAt: d.Data.Post.PublishedAt}
	if failed {
		why := body
		patch = store.OutboundPatch{Status: new("failed"), Error: &why}
	}
	if _, err := s.store.UpdateOutbound(ctx, m.ID, patch); err != nil {
		return "", err
	}
	return "taken", nil
}

// postResultText says how a post did, in the user's language. What a
// network answered is someone else's text: it is cut short and shown as is.
func postResultText(de, failed bool, results []posta.Result, caption string) (string, string) {
	var good, bad []string
	for _, r := range results {
		n := networkName(r.Platform)
		if r.Status == "success" {
			good = append(good, n)
			continue
		}
		if why := strings.TrimSpace(firstText(r.ErrorMessage, r.ErrorCode)); why != "" {
			n += ": " + cutRunes(why, 200)
		}
		bad = append(bad, n)
	}
	quote := ""
	if c := strings.Join(strings.Fields(caption), " "); c != "" {
		quote = "“" + cutRunes(c, 120) + "”"
		if de {
			quote = "„" + cutRunes(c, 120) + "“"
		}
	}
	if failed {
		title := "Your post didn't go out"
		body := "No network took it."
		if de {
			title, body = "Dein Post ist nicht rausgegangen", "Kein Netzwerk hat ihn angenommen."
		}
		if len(bad) > 0 {
			body = strings.Join(bad, "\n")
		}
		if quote != "" {
			body += "\n" + quote
		}
		return title, body
	}
	title, on := "Your post is out", "On %s."
	if de {
		title, on = "Dein Post ist raus", "Auf %s."
	}
	body := quote
	if len(good) > 0 {
		body = strings.TrimSpace(fmt.Sprintf(on, list(good, de)) + " " + quote)
	}
	return title, body
}

// list joins names the way a sentence does: "A, B and C".
func list(names []string, de bool) string {
	and := " and "
	if de {
		and = " und "
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + and + names[len(names)-1]
}

func networkName(platform string) string {
	names := map[string]string{"instagram": "Instagram", "facebook": "Facebook", "tiktok": "TikTok", "youtube": "YouTube",
		"linkedin": "LinkedIn", "threads": "Threads", "pinterest": "Pinterest", "x": "X", "twitter": "X", "bluesky": "Bluesky"}
	if n, ok := names[strings.ToLower(platform)]; ok {
		return n
	}
	return platform
}

func firstText(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

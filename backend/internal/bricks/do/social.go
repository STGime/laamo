package do

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// Social is the Post to social brick (do.social): a card's text, and its
// picture if it has one, become a post on the chosen social accounts,
// through the workspace's Posta account.
//
// A post is public and can't be taken back, so:
//   - a test run posts nothing;
//   - every post is written to outbound_messages before Posta hears of it,
//     and a retried job never makes a second one;
//   - the picture is uploaded first, so a failed upload leaves no post.
type Social struct {
	Store ChatStore
	Keys  *crypto.Keyring
	Posta *posta.Service
}

// MaxCaption is the longest text Posta takes.
const MaxCaption = 2200

// MaxPosts: one brick makes at most this many posts in a run.
const MaxPosts = 5

// needPicture: networks that take no post without a picture or video.
var needPicture = map[string]string{"instagram": "Instagram", "tiktok": "TikTok", "pinterest": "Pinterest", "youtube": "YouTube"}

func (s Social) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return in, nil
	}
	de := rc.Workspace.Locale == "de"
	say := func(en, german string) error {
		if de {
			return queue.Permanent(errors.New(german))
		}
		return queue.Permanent(errors.New(en))
	}
	// A test run shows the posts in the Feed and sends nothing.
	if rc.TestMode {
		rc.Log.Info("test run: nothing is posted to social media")
		return in, nil
	}

	place, user, err := s.account(ctx, rc)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, say("Social posting is off. Switch it on under Me → Social media.",
			"Social-Media-Posten ist aus. Schalte es unter Ich → Social Media ein.")
	}
	channels, err := user.Channels(ctx)
	if err != nil {
		return nil, s.explain(err, de)
	}
	// The chosen accounts, as they are right now.
	var chosen []posta.Channel
	for _, id := range cfg.Strings("channels") {
		i := slices.IndexFunc(channels, func(c posta.Channel) bool { return c.ID == id })
		if i < 0 {
			return nil, say("A social account this brick posts to isn't connected anymore. Open the brick and pick the accounts again.",
				"Ein Social-Media-Konto, auf dem dieser Baustein postet, ist nicht mehr verbunden. Öffne den Baustein und wähle die Konten neu.")
		}
		c := channels[i]
		if !c.IsActive || c.ConnectionError != "" {
			return nil, say(fmt.Sprintf("%s (%s) needs reconnecting: the network withdrew access. Do that under Me → Social media.", name(c), platform(c)),
				fmt.Sprintf("%s (%s) muss neu verbunden werden: Das Netzwerk hat den Zugriff entzogen. Das geht unter Ich → Social Media.", name(c), platform(c)))
		}
		chosen = append(chosen, c)
	}
	if len(chosen) == 0 {
		return nil, say("This brick has no social account to post to. Open it and pick one.",
			"Dieser Baustein hat kein Social-Media-Konto zum Posten. Öffne ihn und wähle eins.")
	}

	cards := in
	if cfg.String("mode") == "one" {
		cards = in[:1]
	}
	if len(cards) > MaxPosts {
		rc.Log.Info("post limit reached: the rest aren't posted", "limit", MaxPosts, "cards", len(cards))
		cards = cards[:MaxPosts]
	}
	when := cfg.String("when")
	if when == "draft" && !pastedAccount(place) {
		// Only people with their own Posta account can open a draft there.
		when = "now"
	}
	now := rc.Now
	if now == nil {
		now = time.Now
	}
	var at *time.Time
	if when == "at" {
		t, err := nextAt(now(), cfg.String("time"), rc.Workspace.Timezone)
		if err != nil {
			return nil, queue.Permanent(err)
		}
		at = &t
	}

	ids := make([]string, len(chosen))
	for i, c := range chosen {
		ids[i] = c.ID
	}
	for i := range cards {
		card := &in[i]
		caption := strings.TrimSpace(card.Text("body"))
		if caption == "" {
			continue // nothing to say: Write it left this card out
		}
		if r := []rune(caption); len(r) > MaxCaption {
			caption = string(r[:MaxCaption-1]) + "…"
		}
		sent, err := s.post(ctx, rc, place, user, card, posta.NewPost{Caption: caption, Channels: ids, At: at, Draft: when == "draft"}, chosen, de, now)
		if err != nil {
			return nil, err
		}
		if sent {
			card.Set("posted_at", now().UTC().Format(time.RFC3339), engine.TrustUser)
		}
	}
	return in, nil
}

// post makes one post at most once.
func (s Social) post(ctx context.Context, rc *engine.RunContext, place store.Place, user *posta.User, card *engine.Card,
	p posta.NewPost, chosen []posta.Channel, de bool, now func() time.Time) (bool, error) {
	idem := fmt.Sprintf("%s:%s:%s", rc.Run.ID, rc.BrickID, card.ID)

	// An earlier attempt of this job may have got somewhere.
	prev, err := s.Store.GetOutboundByKey(ctx, idem)
	switch {
	case err == nil && (prev.Status == "sent" || prev.Status == "draft"):
		return true, nil
	case err == nil && prev.Status == "pending" && prev.ProviderMessageID != nil:
		// The post exists at Posta. If it is still waiting to be sent, send
		// it; publishing the same post again can't make a second one.
		return s.finish(ctx, user, prev, p, de, now)
	case err == nil && prev.Status == "pending":
		// Posta may or may not have made the post, and we hold no id to
		// look it up by. Never risk a second one: stop, and say so.
		rc.Log.Warn("a social post may have been made by an earlier attempt; not posting again", "key", idem)
		msg := "Posta didn't answer while this post was made, so it may or may not have gone out. Look at your social accounts before running this helper again."
		if de {
			msg = "Posta hat nicht geantwortet, während dieser Post angelegt wurde. Er ist vielleicht rausgegangen, vielleicht nicht. Sieh auf deinen Social-Media-Konten nach, bevor du den Helfer noch mal startest."
		}
		return false, queue.Permanent(errors.New(msg))
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return false, err
	}

	// The picture first: nothing is promised yet, so a failure here can
	// simply be tried again.
	if path := card.Text("image"); path != "" {
		id, err := s.upload(ctx, rc, user, path)
		if err != nil {
			return false, s.explain(err, de)
		}
		p.MediaIDs = []string{id}
	} else {
		for _, c := range chosen {
			if n, ok := needPicture[c.Platform]; ok {
				msg := fmt.Sprintf("%s takes no post without a picture. Put “Draw a picture” above this brick, or post to other accounts.", n)
				if de {
					msg = fmt.Sprintf("%s nimmt keinen Post ohne Bild. Setz „Bild malen“ über diesen Baustein oder poste auf anderen Konten.", n)
				}
				return false, queue.Permanent(errors.New(msg))
			}
		}
	}

	m := store.OutboundMessage{WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, RunID: &rc.Run.ID,
		IdempotencyKey: idem, Channel: "posta", Recipient: "place:" + place.ID, Body: &p.Caption, ScheduledFor: p.At}
	if prev.ID != "" { // failed last time, and Posta said so: try again
		m = prev
	} else if err := s.Store.CreateOutbound(ctx, &m); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return false, nil // another attempt is at it right now
		}
		return false, err
	}

	created, err := user.CreatePost(ctx, p)
	if err != nil {
		var pe *posta.Error
		if errors.As(err, &pe) && pe.Status >= 400 && pe.Status < 500 {
			// Posta refused it, so no post was made: a retry may try again.
			// After a timeout or a 5xx nobody knows, and it stays pending.
			_, _ = s.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: new("failed"), Error: new(pe.Error())})
		}
		return false, s.explain(err, de)
	}
	m, err = s.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{ProviderMessageID: &created.ID})
	if err != nil {
		return false, err
	}
	return s.finish(ctx, user, m, p, de, now)
}

// finish sends a post that exists at Posta, unless it is a draft or
// scheduled (Posta sends those itself), and marks the message sent.
func (s Social) finish(ctx context.Context, user *posta.User, m store.OutboundMessage, p posta.NewPost, de bool, now func() time.Time) (bool, error) {
	if !p.Draft && p.At == nil {
		// Only a post that is still waiting: one that went out already
		// answers "can't be published", which must not fail the run.
		if cur, err := user.GetPost(ctx, *m.ProviderMessageID); err != nil {
			return false, s.explain(err, de)
		} else if cur.Status == "draft" || cur.Status == "" {
			if err := user.Publish(ctx, *m.ProviderMessageID); err != nil {
				return false, s.explain(err, de)
			}
		}
	}
	status, sentAt := "sent", now().UTC()
	if p.Draft {
		status = "draft"
	}
	_, err := s.Store.UpdateOutbound(ctx, m.ID, store.OutboundPatch{Status: &status, SentAt: &sentAt})
	return err == nil, err
}

// upload hands a card's picture (from Draw a picture) to Posta.
func (s Social) upload(ctx context.Context, rc *engine.RunContext, user *posta.User, path string) (string, error) {
	// Only this workspace's own pictures: the path comes from a card.
	if rc.Blobs == nil || !strings.HasPrefix(path, "images/"+rc.Run.WorkspaceID+"/") || strings.Contains(path, "..") {
		return "", queue.Permanent(errors.New("this card's picture can't be read"))
	}
	data, err := rc.Blobs.Get(ctx, path)
	if err != nil {
		return "", fmt.Errorf("read picture: %w", err)
	}
	mime := http.DetectContentType(data)
	ext, ok := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}[mime]
	if !ok {
		return "", queue.Permanent(errors.New("this card's picture isn't a JPEG, PNG or WebP"))
	}
	return user.Upload(ctx, "laamo."+ext, mime, data)
}

// account finds the workspace's Posta account. A nil user means social
// posting is off.
func (s Social) account(ctx context.Context, rc *engine.RunContext) (store.Place, *posta.User, error) {
	places, err := s.Store.ListPlaces(ctx, rc.Run.WorkspaceID)
	if err != nil {
		return store.Place{}, nil, err
	}
	i := slices.IndexFunc(places, func(p store.Place) bool { return p.Type == "posta" })
	if i < 0 {
		return store.Place{}, nil, nil
	}
	p := places[i]
	if s.Keys == nil || p.SecretCiphertext == nil {
		return p, nil, queue.Permanent(crypto.ErrNoKey)
	}
	env, err := base64.StdEncoding.DecodeString(*p.SecretCiphertext)
	if err != nil {
		return p, nil, queue.Permanent(err)
	}
	token, err := s.Keys.Decrypt(env, crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		return p, nil, queue.Permanent(errors.New("the key of your Posta account can't be read; switch social posting off and on again"))
	}
	return p, s.Posta.As(string(token)), nil
}

func pastedAccount(p store.Place) bool {
	source, _ := p.Meta["source"].(string)
	return source == "pasted"
}

// explain turns what Posta said into words for the Feed. Errors that a
// retry can't fix are permanent; a timeout or a 5xx is tried again.
func (s Social) explain(err error, de bool) error {
	pick := func(en, german string) error {
		if de {
			return queue.Permanent(errors.New(german))
		}
		return queue.Permanent(errors.New(en))
	}
	var pe *posta.Error
	if errors.Is(err, queue.ErrPermanent) || !errors.As(err, &pe) {
		return err
	}
	switch {
	case errors.Is(err, posta.ErrPlan) && pe.Reason == "trial_expired":
		return pick("Your 5 free days of social posting are over, so nothing was posted.",
			"Deine 5 kostenlosen Tage Social-Media-Posten sind vorbei, darum wurde nichts gepostet.")
	case errors.Is(err, posta.ErrPlan):
		return pick("Social posting is paused, so nothing was posted. See Me → Social media.",
			"Social-Media-Posten ist pausiert, darum wurde nichts gepostet. Sieh unter Ich → Social Media nach.")
	case errors.Is(err, posta.ErrUnauthorized):
		return pick("Posta doesn't accept your account's key anymore. Switch social posting off and on again under Me → Social media.",
			"Posta nimmt den Schlüssel deines Kontos nicht mehr an. Schalte Social-Media-Posten unter Ich → Social Media aus und wieder ein.")
	case pe.Status >= 400 && pe.Status < 500 && pe.Status != http.StatusTooManyRequests:
		why := cut(pe.Message, 200)
		return pick("Posta refused the post: "+why, "Posta hat den Post abgelehnt: "+why)
	}
	return err
}

// nextAt is the next time the clock shows hhmm in the workspace's time
// zone: today if that is still ahead, else tomorrow.
func nextAt(now time.Time, hhmm, zone string) (time.Time, error) {
	if zone == "" {
		zone = "Europe/Berlin"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("unknown time zone %q", zone)
	}
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a time like 09:00", hhmm)
	}
	local := now.In(loc)
	at := time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	// Posta needs a little lead time to schedule.
	if !at.After(local.Add(2 * time.Minute)) {
		at = at.AddDate(0, 0, 1)
	}
	return at.UTC(), nil
}

func name(c posta.Channel) string {
	return firstNonEmpty(c.DisplayName, c.Username, c.Platform)
}

func platform(c posta.Channel) string {
	if n, ok := needPicture[c.Platform]; ok {
		return n
	}
	return title(c.Platform)
}

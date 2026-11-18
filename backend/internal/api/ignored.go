package api

import (
	"context"
	"errors"
	"time"

	guuid "github.com/google/uuid"

	"github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/store"
)

// MaxIgnoredNotesPerDay: Feed notes about ignored emails one helper gets in
// 24 hours. Someone mailing from many addresses shouldn't flood the Feed.
const MaxIgnoredNotesPerDay = 5

// ignoredSpace makes note ids: one note per helper, sender and day.
var ignoredSpace = guuid.MustParse("4c0b6a9e-2f61-4e43-a1d3-69676e6f7265")

// noteIgnored tells the user in the Feed that an email to a helper was
// ignored, and why. The sender never hears about it (no replies to
// strangers: their From may be someone else's address). "Allow this
// sender" is offered only for a signed mail: an unsigned one wouldn't get
// in even with its address on the list.
func (s *Server) noteIgnored(ctx context.Context, h store.Helper, from string, senders []string, signed bool, now time.Time) {
	day := now.UTC().Format(time.DateOnly)
	id := guuid.NewSHA1(ignoredSpace, []byte(h.ID+"/"+from+"/"+day)).String()
	since := now.Add(-24 * time.Hour)
	recent, err := s.store.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: h.WorkspaceID, Types: []string{"problem"}, Limit: 100})
	if err != nil {
		s.log.Error("ignored mail note", "helper", h.ID, "err", err)
		return
	}
	n := 0
	for _, c := range recent {
		if c.ID == id {
			return // already told today
		}
		if c.HelperID != nil && *c.HelperID == h.ID && c.Payload["ignored_mail"] == true && c.CreatedAt.After(since) {
			n++
		}
	}
	if n >= MaxIgnoredNotesPerDay {
		return
	}
	locale := "en"
	if w, err := s.store.GetWorkspace(ctx, h.WorkspaceID); err == nil {
		locale = w.Locale
	}
	listed := mail.Allowed(from, senders)
	title, body := ignoredText(locale == "de", from, listed, signed)
	card := store.FeedCard{ID: id, WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: &h.ID, Type: "problem",
		Title: title, Body: &body,
		Payload: map[string]any{"ignored_mail": true, "from": from, "can_allow": signed && !listed}}
	if err := s.store.CreateFeedCard(ctx, &card); err != nil && !errors.Is(err, store.ErrConflict) {
		s.log.Error("ignored mail note", "helper", h.ID, "err", err)
	}
}

// ignoredText says why an email was ignored, in the user's language.
func ignoredText(de bool, from string, listed, signed bool) (string, string) {
	from = cutRunes(from, 120)
	if de {
		title := "E-Mail ignoriert"
		switch {
		case listed:
			return title, "Angeblich von " + from + " (steht auf der Liste), aber der Mail-Anbieter hat sie nicht signiert. Laamo kann also nicht prüfen, ob sie wirklich von dort kommt. Warst du das, leite aus einem Postfach weiter, das signiert (Gmail, Outlook, GMX und web.de tun das)."
		case signed:
			return title, "Von " + from + ". Diese Adresse steht nicht auf der Absenderliste des Helfers. Erlaube sie, wenn du den Absender kennst."
		}
		return title, "Von " + from + ". Diese Adresse steht nicht auf der Absenderliste des Helfers, und Laamo kann nicht prüfen, ob die E-Mail wirklich von dort kommt."
	}
	title := "Ignored an email"
	switch {
	case listed:
		return title, "It said it was from " + from + ", which is on the list, but its mail provider didn't sign it, so Laamo can't check it really came from there. If it was you, forward from a mailbox that signs its mail (Gmail, Outlook, GMX and web.de do)."
	case signed:
		return title, "From " + from + ". That address isn't on this helper's sender list. Allow it if you know the sender."
	}
	return title, "From " + from + ". That address isn't on this helper's sender list, and Laamo can't check that the email really came from there."
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

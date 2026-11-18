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

// MaxForwardCodesPerDay: Gmail forwarding codes one helper puts in the Feed
// in 24 hours. Anyone can ask Gmail to forward to a helper's address.
const MaxForwardCodesPerDay = 3

// ignoredSpace makes note ids: one note per helper, sender and day.
var ignoredSpace = guuid.MustParse("4c0b6a9e-2f61-4e43-a1d3-69676e6f7265")

// noteIgnored tells the user in the Feed that an email to a helper was
// ignored, and why. The sender never hears about it (no replies to
// strangers: their From may be someone else's address). "Allow this
// sender" is offered only for a signed mail: an unsigned one wouldn't get
// in even with its address on the list.
func (s *Server) noteIgnored(ctx context.Context, h store.Helper, m mail.Message, senders []string, signed bool, now time.Time) {
	from := m.From
	day := now.UTC().Format(time.DateOnly)
	id := guuid.NewSHA1(ignoredSpace, []byte(h.ID+"/"+from+"/"+day)).String()
	if !s.mayNote(ctx, h, id, "ignored_mail", MaxIgnoredNotesPerDay, now) {
		return
	}
	listed := mail.Allowed(from, senders)
	title, body := ignoredText(s.german(ctx, h), from, listed, signed, m.Forwarded)
	card := store.FeedCard{ID: id, WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: &h.ID, Type: "problem",
		Title: title, Body: &body,
		Payload: map[string]any{"ignored_mail": true, "from": from, "can_allow": signed && !listed}}
	if err := s.store.CreateFeedCard(ctx, &card); err != nil && !errors.Is(err, store.ErrConflict) {
		s.log.Error("ignored mail note", "helper", h.ID, "err", err)
	}
}

// german: the helper's workspace speaks German.
func (s *Server) german(ctx context.Context, h store.Helper) bool {
	w, err := s.store.GetWorkspace(ctx, h.WorkspaceID)
	return err == nil && w.Locale == "de"
}

// ignoredText says why an email was ignored, in the user's language. A mail
// that looks auto-forwarded gets a pointer to the forwarding address.
func ignoredText(de bool, from string, listed, signed, forwarded bool) (string, string) {
	title, body := whyIgnored(de, cutRunes(from, 120), listed, signed)
	if forwarded {
		if de {
			body += " Sie sieht automatisch weitergeleitet aus: Trag in deiner Weiterleitung die Weiterleitungsadresse des Helfers ein (auf seiner Seite)."
		} else {
			body += " It looks automatically forwarded: in your forwarding rule, use the helper's forwarding address instead (on its page)."
		}
	}
	return title, body
}

func whyIgnored(de bool, from string, listed, signed bool) (string, string) {
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

// mayNote: the note with this id isn't in the Feed yet, and the helper got
// fewer than max notes of this kind (a payload key) in the last 24 hours.
// The cheap id lookup comes first: repeats from one sender stop there.
func (s *Server) mayNote(ctx context.Context, h store.Helper, id, kind string, maxPerDay int, now time.Time) bool {
	if _, err := s.store.GetFeedCard(ctx, id); err == nil {
		return false // already told
	} else if !errors.Is(err, store.ErrNotFound) {
		s.log.Error("feed note", "helper", h.ID, "err", err)
		return false
	}
	since := now.Add(-24 * time.Hour)
	n := 0
	var before *time.Time
	for { // newest first, back to since
		page, err := s.store.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: h.WorkspaceID, Types: []string{"problem"}, Before: before, Limit: 100})
		if err != nil {
			s.log.Error("feed note", "helper", h.ID, "err", err)
			return false
		}
		for _, c := range page {
			if !c.CreatedAt.After(since) {
				return n < maxPerDay
			}
			if c.HelperID != nil && *c.HelperID == h.ID && c.Payload[kind] != nil && c.Payload[kind] != false {
				if n++; n >= maxPerDay {
					return false
				}
			}
		}
		if len(page) < 100 {
			return true
		}
		before = &page[len(page)-1].CreatedAt
	}
}

// forwardCodeSpace makes note ids: one note per helper and code.
var forwardCodeSpace = guuid.MustParse("9a3e1f52-77c4-4b0a-8e15-666f72776172")

// noteForwardCode puts Gmail's forwarding code in the Feed: the user types
// it into Gmail to finish setting up forwarding to this helper. Someone else
// can ask Gmail to forward to the helper too, so the note says who asked.
func (s *Server) noteForwardCode(ctx context.Context, h store.Helper, code, requester string, now time.Time) {
	id := guuid.NewSHA1(forwardCodeSpace, []byte(h.ID+"/"+code)).String()
	if !s.mayNote(ctx, h, id, "forward_code", MaxForwardCodesPerDay, now) {
		return
	}
	title, body := forwardCodeText(s.german(ctx, h), code, cutRunes(requester, 120))
	card := store.FeedCard{ID: id, WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: &h.ID, Type: "problem",
		Title: title, Body: &body, Payload: map[string]any{"forward_code": code, "from": requester}}
	if err := s.store.CreateFeedCard(ctx, &card); err != nil && !errors.Is(err, store.ErrConflict) {
		s.log.Error("forwarding code note", "helper", h.ID, "err", err)
	}
}

func forwardCodeText(de bool, code, requester string) (string, string) {
	if de {
		who := "Jemand"
		if requester != "" {
			who = requester
		}
		return "Gmail-Weiterleitung bestätigen", who + " möchte E-Mails von Gmail an diesen Helfer weiterleiten. Warst du das, gib in Gmail diesen Code ein: " + code + ". Sonst tu nichts."
	}
	who := "Someone"
	if requester != "" {
		who = requester
	}
	return "Confirm Gmail forwarding", who + " asked Gmail to forward their mail to this helper. If that was you, type this code into Gmail: " + code + ". If not, do nothing."
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

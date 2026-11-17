package waitlist

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"

	lmail "github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/store"
)

// ConsentDirect marks people we invited by hand (not from the form). They
// get the invite email, not the list's news.
const ConsentDirect = "direct-invite"

// Invited reports whether email may use the app.
func (s *Service) Invited(ctx context.Context, email string) (bool, error) {
	e, err := s.Store.WaitlistByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return e.InvitedAt != nil, nil
}

// Next returns up to n people to invite: confirmed, not invited yet, the
// ones who confirmed first first.
func (s *Service) Next(ctx context.Context, n int) ([]store.WaitlistEntry, error) {
	return s.Store.WaitlistToInvite(ctx, n)
}

// Invite emails e their invite and marks them invited. Sending again
// (e.g. after a failed mark) doesn't send a second email for a day: the
// mail provider drops the repeat by its idempotency key.
func (s *Service) Invite(ctx context.Context, e store.WaitlistEntry) (store.WaitlistEntry, error) {
	if e.InvitedAt != nil {
		return e, nil
	}
	msg := inviteMail(e.Locale, s.startLink(e.Email))
	msg.To = e.Email
	msg.IdempotencyKey = "invite-" + e.ID
	if err := s.Mail.Send(ctx, msg); err != nil {
		return e, fmt.Errorf("waitlist: invite email to %s: %w", e.Email, err)
	}
	p := store.WaitlistPatch{InvitedAt: store.Set(s.now())}
	if e.ConfirmedAt == nil {
		// Invited by hand before confirming: keep them (the daily purge
		// deletes unconfirmed entries). Signing in proves the address.
		p.ConfirmedAt, p.TokenHash = store.Set(s.now()), store.Null[string]()
	}
	return s.Store.UpdateWaitlist(ctx, e.ID, p)
}

// InviteEmail invites someone by address, whether or not they are on the
// list (friends, testers, ourselves). lang is used for new entries.
func (s *Service) InviteEmail(ctx context.Context, email, lang string) (store.WaitlistEntry, error) {
	email = CleanEmail(email)
	if email == "" {
		return store.WaitlistEntry{}, ErrEmail
	}
	e, err := s.Store.WaitlistByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		if lang != "de" {
			lang = "en"
		}
		now := s.now()
		e = store.WaitlistEntry{Email: email, Locale: lang, Consent: ConsentDirect, ConfirmedAt: &now}
		for range 3 {
			e.ID, e.RefCode = "", newRefCode()
			if err = s.Store.AddWaitlist(ctx, &e); !errors.Is(err, store.ErrConflict) {
				break
			}
		}
	}
	if err != nil {
		return store.WaitlistEntry{}, err
	}
	return s.Invite(ctx, e)
}

// startLink opens sign-in with the address filled in; ?invite lets it make
// the account.
func (s *Service) startLink(email string) string {
	return strings.TrimRight(s.SiteURL, "/") + "/start?invite&email=" + url.QueryEscape(email)
}

// inviteMail: you're in, one button.
func inviteMail(lang, link string) lmail.Outgoing {
	t := inviteTexts[lang]
	if t.subject == "" {
		t = inviteTexts["en"]
	}
	text := t.hello + "\n\n" + t.body + "\n\n" + link + "\n\n" + t.after + "\n\n" + t.footer + "\n"
	h := html.EscapeString
	page := `<!doctype html><html lang="` + lang + `"><body style="margin:0;padding:24px;font-family:system-ui,-apple-system,'Segoe UI',Roboto,Arial,sans-serif;color:#111111;background:#ffffff;">` +
		`<div style="max-width:480px;margin:0 auto;font-size:16px;line-height:1.5;">` +
		`<p style="font-size:24px;font-weight:800;color:#ff3b5c;margin:0 0 24px;">Laamo</p>` +
		`<p>` + h(t.hello) + `</p><p>` + h(t.body) + `</p>` +
		`<p style="margin:24px 0;"><a href="` + h(link) + `" style="display:inline-block;padding:14px 20px;background:#111111;color:#ffffff;text-decoration:none;font-weight:700;border-radius:8px;">` + h(t.button) + `</a></p>` +
		`<p>` + h(t.after) + `</p>` +
		`<p style="color:#6b6b66;font-size:14px;margin-top:32px;">` + h(t.footer) + `</p>` +
		`</div></body></html>`
	return lmail.Outgoing{Subject: t.subject, Text: text, HTML: page}
}

type inviteText struct{ subject, hello, body, button, after, footer string }

var inviteTexts = map[string]inviteText{
	"en": {
		subject: "You're in: your Laamo invite",
		hello:   "Hi,",
		body:    "your spot is ready. Sign in with this email address and set up your first helper. It takes about five minutes.",
		button:  "Open Laamo",
		after:   "No password needed: we email you a sign-in link.",
		footer:  "Laamo · Eurobase OÜ, Estonia",
	},
	"de": {
		subject: "Du bist dabei: deine Einladung zu Laamo",
		hello:   "Hallo,",
		body:    "dein Platz ist frei. Melde dich mit dieser E-Mail-Adresse an und richte deinen ersten Helfer ein. Das dauert etwa fünf Minuten.",
		button:  "Laamo öffnen",
		after:   "Kein Passwort nötig: Wir schicken dir einen Anmeldelink.",
		footer:  "Laamo · Eurobase OÜ, Estland",
	},
}

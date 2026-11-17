// Package waitlist is the marketing site's waiting list with double opt-in:
// someone enters their email, we send one confirmation link (only to that
// address), and the entry counts once the link was used. Entries never
// confirmed are deleted after 7 days (purge_waitlist, daily cron).
package waitlist

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	lmail "github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/store"
)

// Consent is the version of the sign-up text people agree to; bump it when
// the wording on the form changes.
const Consent = "waitlist-v1"

const (
	// TokenTTL: how long a confirmation link works.
	TokenTTL = 7 * 24 * time.Hour
	// ResendAfter: a second sign-up with the same address within this time
	// sends no new email (nobody can flood someone's inbox through us).
	ResendAfter = 10 * time.Minute
)

// Interests people can tick on the form ("What should a helper do for you?").
var Interests = []string{"customers", "competitors", "social", "news", "emails", "events"}

var (
	ErrEmail = errors.New("that doesn't look like an email address")
	ErrToken = errors.New("this confirmation link doesn't work (anymore)")
)

var refRe = regexp.MustCompile(`^[a-z0-9]{6,12}$`)

// Service runs the waiting list.
type Service struct {
	Store   store.WaitlistStore
	Mail    lmail.Sender
	SiteURL string // e.g. https://laamo.app (the confirmation link points there)
	Log     *slog.Logger
	Now     func() time.Time
}

// Join is what the form sends.
type Join struct {
	Email     string
	Lang      string   // en, de
	Interests []string // from Interests; others are dropped
	Source    string   // page or campaign
	Ref       string   // share code of the person who sent them
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

// CleanEmail lower-cases and checks an address; "" if it isn't one.
func CleanEmail(raw string) string {
	e := strings.ToLower(strings.TrimSpace(raw))
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || len(e) > 254 {
		return ""
	}
	_, domain, _ := strings.Cut(e, "@")
	if !strings.Contains(domain, ".") || strings.HasSuffix(domain, ".") {
		return ""
	}
	return e
}

// Join puts someone on the list (unconfirmed) and emails them the link. The
// outcome looks the same whether the address was new, waiting or already
// confirmed, so the form can't be used to find out who is on the list.
func (s *Service) Join(ctx context.Context, in Join) error {
	email := CleanEmail(in.Email)
	if email == "" {
		return ErrEmail
	}
	lang := "en"
	if in.Lang == "de" {
		lang = "de"
	}
	interests := []string{}
	for _, i := range in.Interests {
		if slices.Contains(Interests, i) && !slices.Contains(interests, i) {
			interests = append(interests, i)
		}
	}

	e, err := s.Store.WaitlistByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return s.add(ctx, email, lang, interests, in)
	case err != nil:
		return err
	case e.ConfirmedAt != nil:
		return nil // already on the list
	case e.TokenSentAt != nil && s.now().Sub(*e.TokenSentAt) < ResendAfter:
		return nil // the link is on its way
	}
	token, hash := newToken()
	sent := s.now()
	e, err = s.Store.UpdateWaitlist(ctx, e.ID, store.WaitlistPatch{Locale: &lang, Interests: interests,
		TokenHash: store.Set(hash), TokenSentAt: store.Set(sent)})
	if err != nil {
		return err
	}
	return s.send(ctx, e, token)
}

func (s *Service) add(ctx context.Context, email, lang string, interests []string, in Join) error {
	token, hash := newToken()
	sent := s.now()
	e := store.WaitlistEntry{Email: email, Locale: lang, Interests: interests, Consent: Consent,
		TokenHash: &hash, TokenSentAt: &sent}
	if src := strings.TrimSpace(in.Source); src != "" {
		e.Source = new(cut(src, 200))
	}
	if ref := strings.ToLower(strings.TrimSpace(in.Ref)); refRe.MatchString(ref) {
		e.ReferredBy = &ref
	}
	for range 3 {
		e.ID, e.RefCode = "", newRefCode()
		err := s.Store.AddWaitlist(ctx, &e)
		if err == nil {
			return s.send(ctx, e, token)
		}
		if !errors.Is(err, store.ErrConflict) {
			return err
		}
		// The same address at the same moment (a double click): fine.
		if _, err := s.Store.WaitlistByEmail(ctx, email); err == nil {
			return nil
		}
		// Otherwise a ref code clash: try another.
	}
	return errors.New("waitlist: no free ref code")
}

// send emails the confirmation link. If that fails, the entry may send
// again right away (the next try isn't held back by ResendAfter).
func (s *Service) send(ctx context.Context, e store.WaitlistEntry, token string) error {
	msg := confirmMail(e.Locale, s.link(e.Locale, token))
	msg.To = e.Email
	msg.IdempotencyKey = "waitlist-" + (*e.TokenHash)[:32]
	if err := s.Mail.Send(ctx, msg); err != nil {
		if _, uerr := s.Store.UpdateWaitlist(ctx, e.ID, store.WaitlistPatch{TokenSentAt: store.Null[time.Time]()}); uerr != nil {
			s.log().Warn("waitlist: reset after failed send", "err", uerr)
		}
		return fmt.Errorf("waitlist: confirmation email: %w", err)
	}
	return nil
}

func (s *Service) link(lang, token string) string {
	path := "/waitlist/confirm"
	if lang == "de" {
		path = "/de" + path
	}
	return strings.TrimRight(s.SiteURL, "/") + path + "?t=" + token
}

// Confirm uses a confirmation link. It returns the entry (its RefCode is the
// person's share code). Using the link again after it worked is ErrToken.
func (s *Service) Confirm(ctx context.Context, token string) (store.WaitlistEntry, error) {
	if len(token) < 20 || len(token) > 100 {
		return store.WaitlistEntry{}, ErrToken
	}
	e, err := s.Store.WaitlistByToken(ctx, hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return store.WaitlistEntry{}, ErrToken
	}
	if err != nil {
		return store.WaitlistEntry{}, err
	}
	// TokenSentAt is cleared when a send failed; created_at still bounds it.
	sentAt := e.CreatedAt
	if e.TokenSentAt != nil {
		sentAt = *e.TokenSentAt
	}
	if s.now().Sub(sentAt) > TokenTTL {
		return store.WaitlistEntry{}, ErrToken
	}
	return s.Store.UpdateWaitlist(ctx, e.ID, store.WaitlistPatch{ConfirmedAt: store.Set(s.now()), TokenHash: store.Null[string]()})
}

func newToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newRefCode: 8 characters without look-alikes (no 0/o, 1/l).
func newRefCode() string {
	const abc = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = abc[int(b[i])%len(abc)]
	}
	return string(b)
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// confirmMail is the double opt-in email: plain words, one link.
func confirmMail(lang, link string) lmail.Outgoing {
	t := texts[lang]
	if t.subject == "" {
		t = texts["en"]
	}
	text := t.hello + "\n\n" + t.ask + "\n\n" + link + "\n\n" + t.after + "\n\n" + t.notYou + "\n\n" + t.footer + "\n"
	h := html.EscapeString
	page := `<!doctype html><html lang="` + lang + `"><body style="margin:0;padding:24px;font-family:system-ui,-apple-system,'Segoe UI',Roboto,Arial,sans-serif;color:#111111;background:#ffffff;">` +
		`<div style="max-width:480px;margin:0 auto;font-size:16px;line-height:1.5;">` +
		`<p style="font-size:24px;font-weight:800;color:#ff3b5c;margin:0 0 24px;">Laamo</p>` +
		`<p>` + h(t.hello) + `</p><p>` + h(t.ask) + `</p>` +
		`<p style="margin:24px 0;"><a href="` + h(link) + `" style="display:inline-block;padding:14px 20px;background:#111111;color:#ffffff;text-decoration:none;font-weight:700;border-radius:8px;">` + h(t.button) + `</a></p>` +
		`<p>` + h(t.after) + `</p>` +
		`<p style="color:#6b6b66;font-size:14px;">` + h(t.notYou) + `</p>` +
		`<p style="color:#6b6b66;font-size:14px;margin-top:32px;">` + h(t.footer) + `</p>` +
		`</div></body></html>`
	return lmail.Outgoing{Subject: t.subject, Text: text, HTML: page}
}

type mailText struct{ subject, hello, ask, button, after, notYou, footer string }

var texts = map[string]mailText{
	"en": {
		subject: "Confirm your spot on the Laamo waiting list",
		hello:   "Hi,",
		ask:     "please confirm that you'd like to join the Laamo waiting list:",
		button:  "Yes, put me on the list",
		after:   "We'll email you when your invite is ready, and now and then about what's new in Laamo.",
		notYou:  "Didn't sign up? Then just ignore this email. We delete unconfirmed addresses after 7 days.",
		footer:  "Laamo · Eurobase OÜ, Estonia",
	},
	"de": {
		subject: "Bestätige deinen Platz auf der Laamo-Warteliste",
		hello:   "Hallo,",
		ask:     "bitte bestätige, dass du auf die Laamo-Warteliste möchtest:",
		button:  "Ja, auf die Liste",
		after:   "Wir schreiben dir, sobald deine Einladung bereit ist, und ab und zu, was es Neues bei Laamo gibt.",
		notYou:  "Du hast dich nicht eingetragen? Dann ignoriere diese E-Mail einfach. Nicht bestätigte Adressen löschen wir nach 7 Tagen.",
		footer:  "Laamo · Eurobase OÜ, Estland",
	},
}

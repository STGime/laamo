package waitlist

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	lmail "github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type outbox struct {
	sent []lmail.Outgoing
	fail bool
}

func (o *outbox) Send(_ context.Context, m lmail.Outgoing) error {
	if o.fail {
		return errors.New("provider down")
	}
	o.sent = append(o.sent, m)
	return nil
}

// token pulls the confirmation token out of the last email's link.
func (o *outbox) token(t *testing.T) string {
	t.Helper()
	m := o.sent[len(o.sent)-1]
	for _, line := range strings.Split(m.Text, "\n") {
		if strings.HasPrefix(line, "https://") {
			u, err := url.Parse(line)
			if err != nil {
				t.Fatal(err)
			}
			return u.Query().Get("t")
		}
	}
	t.Fatalf("no link in %q", m.Text)
	return ""
}

func newService(t *testing.T) (*Service, *outbox, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	box := &outbox{}
	s := &Service{Store: memory.New(), Mail: box, SiteURL: "https://laamo.app/", Now: func() time.Time { return now }}
	return s, box, &now
}

func TestJoinAndConfirm(t *testing.T) {
	s, box, now := newService(t)
	ctx := context.Background()
	err := s.Join(ctx, Join{Email: "  Anna@Example.COM ", Lang: "de", Interests: []string{"social", "x", "social", "customers"},
		Source: "home", Ref: "Abcd2345"})
	if err != nil {
		t.Fatal(err)
	}
	if len(box.sent) != 1 || box.sent[0].To != "anna@example.com" || !strings.Contains(box.sent[0].Subject, "Warteliste") {
		t.Fatalf("sent: %+v", box.sent)
	}
	if !strings.Contains(box.sent[0].Text, "https://laamo.app/de/waitlist/confirm?t=") {
		t.Fatalf("link: %q", box.sent[0].Text)
	}
	e, err := s.Store.WaitlistByEmail(ctx, "anna@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if e.Locale != "de" || strings.Join(e.Interests, ",") != "social,customers" || e.ReferredBy == nil || *e.ReferredBy != "abcd2345" ||
		e.Consent != Consent || e.ConfirmedAt != nil || len(e.RefCode) != 8 {
		t.Fatalf("entry: %+v", e)
	}

	// Signing up again right away sends nothing new.
	if err := s.Join(ctx, Join{Email: "anna@example.com"}); err != nil || len(box.sent) != 1 {
		t.Fatalf("second join: %v, %d sent", err, len(box.sent))
	}
	// Later it sends a fresh link; the old one stops working.
	old := box.token(t)
	*now = now.Add(ResendAfter + time.Minute)
	if err := s.Join(ctx, Join{Email: "anna@example.com", Lang: "de"}); err != nil || len(box.sent) != 2 {
		t.Fatalf("join later: %v, %d sent", err, len(box.sent))
	}
	if _, err := s.Confirm(ctx, old); !errors.Is(err, ErrToken) {
		t.Fatalf("old link: %v", err)
	}

	got, err := s.Confirm(ctx, box.token(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfirmedAt == nil || got.TokenHash != nil || got.RefCode != e.RefCode {
		t.Fatalf("confirmed: %+v", got)
	}
	if _, err := s.Confirm(ctx, box.token(t)); !errors.Is(err, ErrToken) {
		t.Fatalf("link used twice: %v", err)
	}
	// Confirmed people get no more emails from the form.
	*now = now.Add(time.Hour)
	if err := s.Join(ctx, Join{Email: "anna@example.com"}); err != nil || len(box.sent) != 2 {
		t.Fatalf("join when confirmed: %v, %d sent", err, len(box.sent))
	}
}

func TestExpiredLink(t *testing.T) {
	s, box, now := newService(t)
	ctx := context.Background()
	if err := s.Join(ctx, Join{Email: "ben@example.com"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(box.sent[0].Text, "Hi,") {
		t.Fatalf("english mail: %q", box.sent[0].Text)
	}
	*now = now.Add(TokenTTL + time.Hour)
	if _, err := s.Confirm(ctx, box.token(t)); !errors.Is(err, ErrToken) {
		t.Fatalf("expired link: %v", err)
	}
	if _, err := s.Confirm(ctx, "short"); !errors.Is(err, ErrToken) {
		t.Fatalf("junk token: %v", err)
	}
}

func TestBadEmail(t *testing.T) {
	s, box, _ := newService(t)
	for _, bad := range []string{"", "anna", "anna@", "anna@localhost", "Anna <anna@example.com>", "a b@example.com", "anna@example."} {
		if err := s.Join(context.Background(), Join{Email: bad}); !errors.Is(err, ErrEmail) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if len(box.sent) != 0 {
		t.Fatalf("sent %d", len(box.sent))
	}
}

func TestSendFails(t *testing.T) {
	s, box, _ := newService(t)
	ctx := context.Background()
	box.fail = true
	if err := s.Join(ctx, Join{Email: "cara@example.com"}); err == nil {
		t.Fatal("no error when the email couldn't be sent")
	}
	// The next try sends at once (not held back by ResendAfter).
	box.fail = false
	if err := s.Join(ctx, Join{Email: "cara@example.com"}); err != nil || len(box.sent) != 1 {
		t.Fatalf("retry: %v, %d sent", err, len(box.sent))
	}
	e, _ := s.Store.WaitlistByEmail(ctx, "cara@example.com")
	if e.TokenSentAt == nil {
		t.Fatalf("token_sent_at not set after the retry: %+v", e)
	}
}

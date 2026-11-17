package waitlist

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// confirmed puts email on the list and confirms it.
func confirmed(t *testing.T, s *Service, box *outbox, email, lang string) {
	t.Helper()
	ctx := context.Background()
	if err := s.Join(ctx, Join{Email: email, Lang: lang}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Confirm(ctx, box.token(t)); err != nil {
		t.Fatal(err)
	}
}

func TestInviteNextInOrder(t *testing.T) {
	s, box, now := newService(t)
	ctx := context.Background()
	confirmed(t, s, box, "first@example.com", "de")
	*now = now.Add(time.Minute)
	confirmed(t, s, box, "second@example.com", "en")
	*now = now.Add(time.Minute)
	if err := s.Join(ctx, Join{Email: "unconfirmed@example.com"}); err != nil {
		t.Fatal(err)
	}

	next, err := s.Next(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 2 || next[0].Email != "first@example.com" || next[1].Email != "second@example.com" {
		t.Fatalf("next = %+v", next)
	}
	if ok, _ := s.Invited(ctx, "first@example.com"); ok {
		t.Fatal("invited before the invite")
	}

	sent := len(box.sent)
	e, err := s.Invite(ctx, next[0])
	if err != nil {
		t.Fatal(err)
	}
	if e.InvitedAt == nil {
		t.Fatal("not marked invited")
	}
	m := box.sent[sent]
	if m.To != "first@example.com" || !strings.Contains(m.Subject, "Einladung") ||
		!strings.Contains(m.Text, "https://laamo.app/start?invite&email=first%40example.com") || m.IdempotencyKey != "invite-"+e.ID {
		t.Fatalf("invite mail = %+v", m)
	}
	if ok, _ := s.Invited(ctx, " First@Example.com"); !ok {
		t.Fatal("not invited after the invite")
	}
	// Inviting again sends nothing.
	if _, err := s.Invite(ctx, e); err != nil || len(box.sent) != sent+1 {
		t.Fatalf("second invite: err %v, %d mails", err, len(box.sent)-sent)
	}
	next, _ = s.Next(ctx, 10)
	if len(next) != 1 || next[0].Email != "second@example.com" {
		t.Fatalf("next after invite = %+v", next)
	}
}

func TestInviteFailedSendStaysUninvited(t *testing.T) {
	s, box, _ := newService(t)
	ctx := context.Background()
	confirmed(t, s, box, "anna@example.com", "en")
	next, _ := s.Next(ctx, 1)
	box.fail = true
	if _, err := s.Invite(ctx, next[0]); err == nil {
		t.Fatal("want an error")
	}
	if ok, _ := s.Invited(ctx, "anna@example.com"); ok {
		t.Fatal("marked invited without an email")
	}
}

func TestInviteEmail(t *testing.T) {
	s, box, _ := newService(t)
	ctx := context.Background()
	// Not on the list: added as a direct invite.
	e, err := s.InviteEmail(ctx, "Friend@Example.com", "de")
	if err != nil {
		t.Fatal(err)
	}
	if e.Consent != ConsentDirect || e.InvitedAt == nil || e.ConfirmedAt == nil || e.Locale != "de" {
		t.Fatalf("entry = %+v", e)
	}
	if ok, _ := s.Invited(ctx, "friend@example.com"); !ok || len(box.sent) != 1 {
		t.Fatal("direct invite didn't invite")
	}
	// On the list, not confirmed yet: the invite counts anyway.
	if err := s.Join(ctx, Join{Email: "waiting@example.com"}); err != nil {
		t.Fatal(err)
	}
	if e, err := s.InviteEmail(ctx, "waiting@example.com", "en"); err != nil || e.ConfirmedAt == nil {
		t.Fatalf("invite of an unconfirmed entry: %+v, %v", e, err)
	}
	if ok, _ := s.Invited(ctx, "waiting@example.com"); !ok {
		t.Fatal("listed person not invited")
	}
	if _, err := s.InviteEmail(ctx, "nope", "en"); !errors.Is(err, ErrEmail) {
		t.Fatalf("bad email: %v", err)
	}
}

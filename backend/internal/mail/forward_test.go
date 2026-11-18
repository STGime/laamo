package mail_test

import (
	"context"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// Every email helper gets a forwarding address; it stays the same when the
// helper is saved again, helpers from before get one, and a new key
// replaces the old one.
func TestForwardingAddress(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := store.Helper{ID: store.NewID(), WorkspaceID: store.NewID(), UserID: store.NewID(), Name: "Rechnungen"}
	stack := catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{{ID: "b1", Type: "when.email"}}}
	if err := mail.SyncAddress(ctx, st, h, stack, "in.laamo.app"); err != nil {
		t.Fatal(err)
	}
	a, err := mail.HelperAddresses(ctx, st, h.ID)
	local, _, _ := strings.Cut(a.Address, "@")
	if err != nil || !strings.HasPrefix(a.Forward, local+"+") || !strings.HasSuffix(a.Forward, "@in.laamo.app") || len(a.Forward) != len(a.Address)+13 {
		t.Fatalf("%+v %v", a, err)
	}
	_ = mail.SyncAddress(ctx, st, h, stack, "in.laamo.app")
	if again, _ := mail.HelperAddresses(ctx, st, h.ID); again != a {
		t.Fatalf("changed on save: %+v → %+v", a, again)
	}
	ts, _ := st.HelperTriggers(ctx, h.ID)
	_, tag, _ := mail.Recipient(a.Forward)
	if !mail.IsForwardKey(ts[0], tag) || !mail.IsForwardKey(ts[0], strings.ToUpper(tag)) || mail.IsForwardKey(ts[0], "news") || mail.IsForwardKey(ts[0], "") {
		t.Fatal("IsForwardKey")
	}

	n, err := mail.NewForwardKey(ctx, st, h.ID)
	if err != nil || n.Address != a.Address || n.Forward == a.Forward || n.Forward == "" {
		t.Fatalf("new key: %+v %v", n, err)
	}
	ts, _ = st.HelperTriggers(ctx, h.ID)
	if mail.IsForwardKey(ts[0], tag) {
		t.Fatal("old key still works")
	}

	// A helper from before forwarding addresses: saving it adds one.
	old := store.Helper{ID: store.NewID(), WorkspaceID: h.WorkspaceID, UserID: h.UserID, Name: "Alt"}
	addr := "alt.abcdef@in.laamo.app"
	if err := st.CreateTrigger(ctx, &store.Trigger{WorkspaceID: old.WorkspaceID, UserID: old.UserID, HelperID: old.ID, Type: store.TriggerEmail, InboundAddress: &addr}); err != nil {
		t.Fatal(err)
	}
	_ = mail.SyncAddress(ctx, st, old, stack, "in.laamo.app")
	if oa, _ := mail.HelperAddresses(ctx, st, old.ID); oa.Address != addr || !strings.HasPrefix(oa.Forward, "alt.abcdef+") {
		t.Fatalf("old helper: %+v", oa)
	}
}

func TestGmailForwardCode(t *testing.T) {
	m := mail.Message{From: mail.GmailForwarding,
		Subject: "(#123456789) Gmail Forwarding Confirmation - Receive Mail from anna@gmail.com",
		Text:    "anna@gmail.com has requested to automatically forward mail to your email address rechnungen.x7k2qa+k3m9@in.laamo.app.\nConfirmation code: 123456789"}
	if code, who, ok := mail.GmailForwardCode(m, "in.laamo.app"); !ok || code != "123456789" || who != "anna@gmail.com" {
		t.Fatalf("%q %q %v", code, who, ok)
	}
	// German, code only in the text, requester only in the text.
	m = mail.Message{Subject: "Gmail-Weiterleitungsbestätigung", Text: "rechnungen.x7k2qa@in.laamo.app: anna@gmail.com möchte weiterleiten. Bestätigungscode: 987654321"}
	if code, who, ok := mail.GmailForwardCode(m, "in.laamo.app"); !ok || code != "987654321" || who != "anna@gmail.com" {
		t.Fatalf("de: %q %q %v", code, who, ok)
	}
	if _, _, ok := mail.GmailForwardCode(mail.Message{Subject: "Your Gmail tips"}, "in.laamo.app"); ok {
		t.Fatal("code without a code")
	}
}

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/engine"
	qmem "github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

const secret = "test-inbound-secret"

func mailFrom(from, subject string) []byte {
	return []byte("From: " + from + "\r\nTo: x@in.laamo.app\r\nSubject: " + subject +
		"\r\nMessage-ID: <" + subject + "@example.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nKönnen wir uns treffen?\r\n")
}

// deliver posts a mail the way the Email Worker does.
func deliver(t *testing.T, url, to string, raw []byte, sign string) (int, string) {
	t.Helper()
	ts := time.Now().Unix()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/api/inbound/email", bytes.NewReader(raw))
	req.Header.Set("X-Laamo-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Laamo-Signature", hex.EncodeToString(api.InboundSignature(sign, ts, raw)))
	req.Header.Set("X-Laamo-To", to)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct{ Outcome string }
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out.Outcome
}

func TestInboundMail(t *testing.T) {
	st := memory.New()
	q := qmem.New()
	blobs := &engine.MemoryBlobs{}
	cfg := config.Config{InboundDomain: "in.laamo.app", InboundSecret: secret}
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: cfg, Store: st, Auth: fakeAuth{"anna": anna}, Queue: q, Blobs: blobs}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Termine", Status: "active"})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.email"}, {ID: "b2", Type: "think.summarize"}}})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)

	// Saving the stack gives the helper its address; saving again keeps it.
	c := e.client(t, "anna")
	got := must(c.GetHelperStackWithResponse(ctx, uuidOf(h.ID)))
	saved := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: got.JSON200.Stack, Version: 1}))
	if saved.StatusCode() != 200 || saved.JSON200.EmailAddress == nil {
		t.Fatalf("save: %d %s", saved.StatusCode(), saved.Body)
	}
	addr := *saved.JSON200.EmailAddress
	again := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: got.JSON200.Stack, Version: 2}))
	if again.JSON200.EmailAddress == nil || *again.JSON200.EmailAddress != addr {
		t.Fatalf("address changed: %s → %v", addr, again.JSON200.EmailAddress)
	}
	if d := must(c.GetHelperWithResponse(ctx, uuidOf(h.ID))); d.JSON200.Helper.EmailAddress == nil || *d.JSON200.Helper.EmailAddress != addr {
		t.Fatal("helper details lack the address")
	}

	// Only from me by default: a stack that never had a sender list gets
	// the user's address, and an unsigned mail claiming to be from them
	// (anyone can write any From) doesn't start a run.
	cur := must(c.GetHelperStackWithResponse(ctx, uuidOf(h.ID))).JSON200.Stack
	if s, _ := json.Marshal(cur.Bricks[0].Config); string(s) != `{"senders":["anna@example.com"]}` {
		t.Fatalf("senders not defaulted to the user: %s", s)
	}
	if code, out := deliver(t, srv.URL, "<"+addr+">", mailFrom("Anna <anna@example.com>", "termin"), secret); code != 202 || out != "sender_not_allowed" {
		t.Fatalf("unsigned mail from the owner's address: %d %s", code, out)
	}
	// The user hears about it in the Feed (the sender doesn't): once per
	// sender and day, without Allow (an unsigned mail wouldn't get in).
	notes := func() []store.FeedCard {
		var out []store.FeedCard
		all, _ := st.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: ws.ID})
		for _, fc := range all {
			if fc.Payload["ignored_mail"] == true {
				out = append(out, fc)
			}
		}
		return out
	}
	_, _ = deliver(t, srv.URL, "<"+addr+">", mailFrom("Anna <anna@example.com>", "termin again"), secret)
	if n := notes(); len(n) != 1 || n[0].Type != "problem" || n[0].HelperID == nil || *n[0].HelperID != h.ID ||
		n[0].Payload["from"] != "anna@example.com" || n[0].Payload["can_allow"] != false || !strings.Contains(*n[0].Body, "nicht signiert") {
		t.Fatalf("notes: %+v", n)
	}
	// Mails from many addresses don't flood the Feed.
	for i := range 8 {
		_, _ = deliver(t, srv.URL, "<"+addr+">", mailFrom(fmt.Sprintf("x%d@spam.example", i), "hi"), secret)
	}
	if n := notes(); len(n) != api.MaxIgnoredNotesPerDay {
		t.Fatalf("notes after spam: %d", len(n))
	}
	// Cleared on purpose: anyone may email the helper.
	cur.Bricks[0].Config = map[string]any{"senders": []any{}}
	if r := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: cur, Version: 3})); r.StatusCode() != 200 {
		t.Fatalf("save open: %d %s", r.StatusCode(), r.Body)
	}

	// A mail: one run with the email, stored in Storage.
	if code, out := deliver(t, srv.URL, "<"+addr+">", mailFrom("Anna <anna@example.com>", "termin"), secret); code != 202 || out != "run_started" {
		t.Fatalf("deliver: %d %s", code, out)
	}
	runs, _ := st.ListRuns(ctx, store.RunFilter{HelperID: h.ID})
	if len(runs) != 1 || runs[0].TriggerType != "email" || runs[0].TriggerData["from"] != "anna@example.com" || runs[0].TriggerData["text"] != "Können wir uns treffen?" ||
		runs[0].TriggerData["from_signed"] != "no" { // no DKIM signature

		t.Fatalf("runs: %+v", runs)
	}
	if data, err := blobs.Get(ctx, runs[0].TriggerData["mail"].(string)); err != nil || !bytes.Contains(data, []byte("Subject: termin")) {
		t.Fatalf("stored: %v", err)
	}
	if len(q.Jobs()) != 1 {
		t.Fatalf("jobs: %d", len(q.Jobs()))
	}
	// The same mail again (Cloudflare retried): still one run.
	if code, out := deliver(t, srv.URL, addr, mailFrom("Anna <anna@example.com>", "termin"), secret); code != 202 || out != "duplicate" {
		t.Fatalf("again: %d %s", code, out)
	}

	// Refused: wrong signature, unknown address, another domain, too big.
	if code, _ := deliver(t, srv.URL, addr, mailFrom("a@b.de", "x"), "wrong"); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if code, _ := deliver(t, srv.URL, "nobody.aaaaaa@in.laamo.app", mailFrom("a@b.de", "x"), secret); code != 404 {
		t.Fatalf("unknown address: %d", code)
	}
	if code, _ := deliver(t, srv.URL, "termine@gmail.com", mailFrom("a@b.de", "x"), secret); code != 404 {
		t.Fatalf("other domain: %d", code)
	}
	if code, _ := deliver(t, srv.URL, addr, []byte("not an email"), secret); code != 422 {
		t.Fatalf("not an email: %d", code)
	}

	// Only from these senders: the address must be listed and signed by its
	// domain; this one is listed but unsigned (anyone could fake it).
	raw, _ = json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.email", Config: map[string]any{"senders": []any{"anna@example.com"}}}, {ID: "b2", Type: "think.summarize"}}})
	v2 := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 3, Stack: raw})
	h.CurrentVersionID = &v2.ID
	st.PutHelper(h)
	if code, out := deliver(t, srv.URL, addr, mailFrom("anna@example.com", "faked"), secret); code != 202 || out != "sender_not_allowed" {
		t.Fatalf("unsigned sender: %d %s", code, out)
	}

	// Switched off: 410, the sender learns it didn't arrive.
	h.Status = "paused"
	st.PutHelper(h)
	if code, out := deliver(t, srv.URL, addr, mailFrom("a@b.de", "off"), secret); code != 410 || out != "helper_off" {
		t.Fatalf("paused: %d %s", code, out)
	}

	// The daily limit.
	h.Status = "active"
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)
	for i := range api.MaxMailsPerDay {
		r := store.Run{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, VersionID: v.ID, TriggerType: "email", Status: "done"}
		_ = i
		if err := st.CreateRun(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	if code, out := deliver(t, srv.URL, addr, mailFrom("a@b.de", "many"), secret); code != 202 || out != "daily_limit" {
		t.Fatalf("limit: %d %s", code, out)
	}

	// Email this helper removed: the address stops working.
	tap := got.JSON200.Stack
	tap.Bricks[0] = api.StackBrick{Id: "b1", Type: "when.tap", Config: map[string]any{}}
	if r := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: tap, Version: 1})); r.StatusCode() != 200 || r.JSON200.EmailAddress != nil {
		t.Fatalf("tap: %d %s", r.StatusCode(), r.Body)
	}
	if code, _ := deliver(t, srv.URL, addr, mailFrom("a@b.de", "later"), secret); code != 404 {
		t.Fatalf("after removing: %d", code)
	}
}

func uuidOf(id string) openapi_types.UUID { return wsID(id) }

// Auto-forwarded mail keeps its original sender, so it can't pass Only from
// these senders; sent to the helper's forwarding address it gets in anyway.
// A new forwarding address shuts the old one.
func TestForwardingAddress(t *testing.T) {
	st := memory.New()
	cfg := config.Config{InboundDomain: "in.laamo.app", InboundSecret: secret}
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: cfg, Store: st, Auth: fakeAuth{"anna": anna}, Queue: qmem.New(), Blobs: &engine.MemoryBlobs{}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Rechnungen", Status: "active"})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.email"}, {ID: "b2", Type: "think.summarize"}}})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)

	c := e.client(t, "anna")
	got := must(c.GetHelperStackWithResponse(ctx, uuidOf(h.ID)))
	saved := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: got.JSON200.Stack, Version: 1}))
	if saved.StatusCode() != 200 || saved.JSON200.EmailAddress == nil || saved.JSON200.ForwardAddress == nil {
		t.Fatalf("save: %d %s", saved.StatusCode(), saved.Body)
	}
	addr, fwd := *saved.JSON200.EmailAddress, *saved.JSON200.ForwardAddress
	if d := must(c.GetHelperWithResponse(ctx, uuidOf(h.ID))); d.JSON200.Helper.ForwardAddress == nil || *d.JSON200.Helper.ForwardAddress != fwd {
		t.Fatal("helper details lack the forwarding address")
	}
	invoice := func(subject string) []byte {
		return []byte("From: Stadtwerke <rechnung@stadtwerke.example>\r\nTo: anna@example.com\r\nX-Forwarded-To: " + fwd +
			"\r\nSubject: " + subject + "\r\nMessage-ID: <" + subject + "@stadtwerke.example>\r\n\r\nIhre Rechnung\r\n")
	}

	// To the plain address: ignored (not from Anna), and the note points to
	// the forwarding address since it looks auto-forwarded.
	if code, out := deliver(t, srv.URL, addr, invoice("r1"), secret); code != 202 || out != "sender_not_allowed" {
		t.Fatalf("plain address: %d %s", code, out)
	}
	all, _ := st.ListFeedCards(ctx, store.FeedFilter{WorkspaceID: ws.ID})
	if len(all) != 1 || !strings.Contains(*all[0].Body, "Weiterleitungsadresse") {
		t.Fatalf("note: %+v", all)
	}
	// Some other tag isn't the key.
	local, domain, _ := strings.Cut(addr, "@")
	if _, out := deliver(t, srv.URL, local+"+news@"+domain, invoice("r2"), secret); out != "sender_not_allowed" {
		t.Fatalf("other tag: %s", out)
	}
	// To the forwarding address: a run.
	if code, out := deliver(t, srv.URL, "<"+strings.ToUpper(fwd)+">", invoice("r3"), secret); code != 202 || out != "run_started" {
		t.Fatalf("forwarding address: %d %s", code, out)
	}
	runs, _ := st.ListRuns(ctx, store.RunFilter{HelperID: h.ID})
	if len(runs) != 1 || runs[0].TriggerData["from"] != "rechnung@stadtwerke.example" || runs[0].TriggerData["from_signed"] != "no" {
		t.Fatalf("runs: %+v", runs)
	}

	// A new forwarding address: the old one is just the plain address now.
	n := must(c.NewForwardAddressWithResponse(ctx, uuidOf(h.ID)))
	if n.StatusCode() != 200 || n.JSON200.ForwardAddress == nil || *n.JSON200.ForwardAddress == fwd || *n.JSON200.EmailAddress != addr {
		t.Fatalf("new: %d %s", n.StatusCode(), n.Body)
	}
	if _, out := deliver(t, srv.URL, fwd, invoice("r4"), secret); out != "sender_not_allowed" {
		t.Fatalf("old forwarding address: %s", out)
	}
	if _, out := deliver(t, srv.URL, *n.JSON200.ForwardAddress, invoice("r5"), secret); out != "run_started" {
		t.Fatalf("new forwarding address: %s", out)
	}
	if r := must(c.NewForwardAddressWithResponse(ctx, uuidOf(store.NewID()))); r.StatusCode() != 404 {
		t.Fatalf("unknown helper: %d", r.StatusCode())
	}
}

// Another workspace member saving the owner's email helper doesn't make it
// "only from" that member.
func TestOnlyMeIsTheOwner(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: config.Config{InboundDomain: "in.laamo.app"}, Store: st,
		Auth: fakeAuth{"anna": anna, "ben": ben}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: ben.ID, Role: store.RoleMember})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Termine", Status: "active"})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.email"}, {ID: "b2", Type: "think.summarize"}}})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)

	c := e.client(t, "ben")
	got := must(c.GetHelperStackWithResponse(ctx, uuidOf(h.ID)))
	saved := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: got.JSON200.Stack, Version: 1}))
	if saved.StatusCode() != 200 {
		t.Fatalf("save: %d %s", saved.StatusCode(), saved.Body)
	}
	if _, set := saved.JSON200.Stack.Bricks[0].Config["senders"]; set {
		t.Fatalf("senders set by a member: %v", saved.JSON200.Stack.Bricks[0].Config)
	}
}

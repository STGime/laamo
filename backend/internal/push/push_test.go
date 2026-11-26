package push_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/STGime/laamo/backend/internal/push"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

var b64 = base64.RawURLEncoding

// browser is a test double for a browser and its push service: it makes a
// subscription, takes pushes over HTTP and decrypts them the way a browser
// does (RFC 8291), so the test reads what a phone would show.
type browser struct {
	t      *testing.T
	srv    *httptest.Server
	key    *ecdh.PrivateKey
	auth   []byte
	status int // what the push service answers (0 = 201)

	mu   sync.Mutex
	got  []push.Message
	reqs []*http.Request
}

func newBrowser(t *testing.T) *browser {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b := &browser{t: t, key: key, auth: make([]byte, 16)}
	_, _ = rand.Read(b.auth)
	b.srv = httptest.NewTLSServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *browser) sub(userID string) store.PushSubscription {
	return store.PushSubscription{UserID: userID, Endpoint: b.srv.URL + "/send/" + store.NewID(),
		P256dh: b64.EncodeToString(b.key.PublicKey().Bytes()), Auth: b64.EncodeToString(b.auth)}
}

func (b *browser) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	defer b.mu.Unlock()
	// Firefox on phones: its push service refuses more than about 2000 bytes.
	if len(body) > 2000 {
		b.t.Errorf("push is %d bytes, too large for Firefox on phones", len(body))
	}
	b.reqs = append(b.reqs, r)
	if b.status != 0 {
		w.WriteHeader(b.status)
		return
	}
	var m push.Message
	if err := json.Unmarshal(b.decrypt(body), &m); err != nil {
		b.t.Errorf("push body: %v", err)
	}
	b.got = append(b.got, m)
	w.WriteHeader(http.StatusCreated)
}

// decrypt opens an aes128gcm message: salt(16) rs(4) idlen(1) keyid, then
// the ciphertext.
func (b *browser) decrypt(body []byte) []byte {
	b.t.Helper()
	if len(body) < 21 || len(body) < 21+int(body[20]) {
		b.t.Fatalf("push body too short: %d bytes", len(body))
	}
	salt, serverPub, ct := body[:16], body[21:21+int(body[20])], body[21+int(body[20]):]
	pub, err := ecdh.P256().NewPublicKey(serverPub)
	if err != nil {
		b.t.Fatal(err)
	}
	secret, err := b.key.ECDH(pub)
	if err != nil {
		b.t.Fatal(err)
	}
	info := append([]byte("WebPush: info\x00"), b.key.PublicKey().Bytes()...)
	info = append(info, serverPub...)
	derive := func(secret, salt []byte, info string, n int) []byte {
		out, err := hkdf.Key(sha256.New, secret, salt, info, n)
		if err != nil {
			b.t.Fatal(err)
		}
		return out
	}
	ikm := derive(secret, b.auth, string(info), 32)
	block, _ := aes.NewCipher(derive(ikm, salt, "Content-Encoding: aes128gcm\x00", 16))
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, derive(ikm, salt, "Content-Encoding: nonce\x00", 12), ct, nil)
	if err != nil {
		b.t.Fatalf("a browser could not decrypt this push: %v", err)
	}
	// Padding: the text, a 0x02, then zeros.
	plain = []byte(strings.TrimRight(string(plain), "\x00"))
	return plain[:len(plain)-1]
}

func (b *browser) messages() []push.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]push.Message(nil), b.got...)
}

func sender(t *testing.T, b *browser) *push.Sender {
	t.Helper()
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	return &push.Sender{PublicKey: public, PrivateKey: private, Subject: "https://laamo.app",
		HTTP: b.srv.Client(), Allow: func(string) bool { return true }}
}

func TestSend(t *testing.T) {
	b := newBrowser(t)
	s := sender(t, b)
	sub := b.sub("u1")
	long := strings.Repeat("Ein ziemlich langer Satz über Brötchen. ", 10)
	if err := s.Send(context.Background(), sub, push.Message{Title: "Beleg-Helfer", Body: long, URL: "/feed", Tag: "h1"}, "topic1"); err != nil {
		t.Fatal(err)
	}
	got := b.messages()
	if len(got) != 1 || got[0].Title != "Beleg-Helfer" || got[0].URL != "/feed" || got[0].Tag != "h1" {
		t.Fatalf("the browser got %+v", got)
	}
	if n := len([]rune(got[0].Body)); n > 160 || !strings.HasSuffix(got[0].Body, "…") {
		t.Fatalf("body not shortened: %d characters", n)
	}

	r := b.reqs[0]
	if r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("TTL") != "172800" || r.Header.Get("Topic") != "topic1" {
		t.Fatalf("headers: %v", r.Header)
	}
	// The push service checks who sent it: a token signed with our key,
	// made out to this push service.
	token, key, ok := strings.Cut(strings.TrimPrefix(r.Header.Get("Authorization"), "vapid t="), ", k=")
	if !ok || key != s.PublicKey {
		t.Fatalf("Authorization: %q", r.Header.Get("Authorization"))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token: %q", token)
	}
	raw, _ := b64.DecodeString(s.PublicKey)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := b64.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the token isn't signed with our key")
	}
	var claims struct{ Aud, Sub string }
	body, _ := b64.DecodeString(parts[1])
	_ = json.Unmarshal(body, &claims)
	if claims.Aud != b.srv.URL || claims.Sub != "https://laamo.app" {
		t.Fatalf("claims: %+v", claims)
	}
}

// The longest message we can make still fits (every character 4 bytes).
func TestSendLongest(t *testing.T) {
	b := newBrowser(t)
	s := sender(t, b)
	m := push.Message{Title: strings.Repeat("🥐", 80), Body: strings.Repeat("🥐", 300), URL: "/feed", Tag: store.NewID()}
	if err := s.Send(context.Background(), b.sub("u1"), m, ""); err != nil {
		t.Fatal(err)
	}
	if got := b.messages(); len(got) != 1 || len([]rune(got[0].Body)) != 160 {
		t.Fatalf("the browser got %+v", got)
	}
}

func TestSendRefusals(t *testing.T) {
	b := newBrowser(t)
	s := sender(t, b)
	ctx := context.Background()
	for status, gone := range map[int]bool{http.StatusGone: true, http.StatusNotFound: true, http.StatusTooManyRequests: false} {
		b.status = status
		err := s.Send(ctx, b.sub("u1"), push.Message{Title: "x"}, "")
		if err == nil || errors.Is(err, push.ErrGone) != gone {
			t.Fatalf("%d: %v", status, err)
		}
	}
	// Without the test override, only the browsers' push services get mail.
	s.Allow = nil
	b.status = 0
	n := len(b.reqs)
	if err := s.Send(ctx, b.sub("u1"), push.Message{Title: "x"}, ""); err == nil || len(b.reqs) != n {
		t.Fatalf("sent to an unknown host: %v", err)
	}
}

func TestKnownService(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                    true,
		"https://web.push.apple.com/QGk":                             true,
		"https://updates.push.services.mozilla.com/wpush/v2/gAAA":    true,
		"https://wns2-par02p.notify.windows.com/w/?token=x":          true,
		"http://fcm.googleapis.com/fcm/send/abc":                     false, // not https
		"https://fcm.googleapis.com.evil.example/x":                  false,
		"https://evilpush.apple.com/x":                               false,
		"https://user@fcm.googleapis.com/x":                          false,
		"https://fcm.googleapis.com:8443/x":                          false,
		"https://169.254.169.254/latest/meta-data":                   false,
		"https://laamo-api.functions.fnc.fr-par.scw.cloud/api/admin": false,
		"": false,
	} {
		if got := push.KnownService(endpoint); got != want {
			t.Errorf("%q: %v, want %v", endpoint, got, want)
		}
	}
}

// world: a workspace owner with one browser, a helper, and the wrapped store.
type world struct {
	st     store.Store
	mem    *memory.Store
	b      *browser
	helper store.Helper
}

const owner = "aaaaaaaa-0000-0000-0000-000000000001"

func newWorld(t *testing.T) world {
	t.Helper()
	b := newBrowser(t)
	mem := memory.New()
	ws := mem.PutWorkspace(store.Workspace{UserID: owner, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	h := store.Helper{WorkspaceID: ws.ID, UserID: owner, Name: "Beleg-Helfer"}
	if err := mem.CreateHelper(context.Background(), &h, &store.HelperVersion{}); err != nil {
		t.Fatal(err)
	}
	sub := b.sub(owner)
	if err := mem.SavePushSubscription(context.Background(), &sub); err != nil {
		t.Fatal(err)
	}
	n := &push.Notifier{Store: mem, Sender: sender(t, b)}
	return world{st: push.Wrap(mem, n), mem: mem, b: b, helper: h}
}

func (w world) run(t *testing.T, trigger string, test bool) store.Run {
	t.Helper()
	r := store.Run{WorkspaceID: w.helper.WorkspaceID, UserID: owner, HelperID: w.helper.ID,
		VersionID: *w.helper.CurrentVersionID, TriggerType: trigger, TestMode: test}
	if err := w.st.CreateRun(context.Background(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func (w world) card(t *testing.T, r *store.Run, typ, title, body string) error {
	t.Helper()
	c := store.FeedCard{WorkspaceID: w.helper.WorkspaceID, UserID: owner, HelperID: &w.helper.ID, Type: typ, Title: title}
	if body != "" {
		c.Body = &body
	}
	if r != nil {
		c.RunID = &r.ID
	}
	return w.st.CreateFeedCard(context.Background(), &c)
}

func TestFeedCardsAreAnnounced(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	// A helper ran on its own and has a result: the phone hears about it.
	sched := w.run(t, store.TriggerSchedule, false)
	if err := w.card(t, &sched, "result", "3 neue Ausschreibungen", "…"); err != nil {
		t.Fatal(err)
	}
	// An email came in and the helper needs a check: title is the helper's
	// name, so the question is the text.
	mail := w.run(t, store.TriggerEmail, false)
	step := store.Step{WorkspaceID: mail.WorkspaceID, UserID: owner, RunID: mail.ID, BrickID: "b3", BrickType: "ask.check", Position: 2}
	if err := w.mem.CreateStep(ctx, &step); err != nil {
		t.Fatal(err)
	}
	if err := w.card(t, &mail, "approval", "Beleg-Helfer", "Stimmt dieser Beleg?"); err != nil {
		t.Fatal(err)
	}
	got := w.b.messages()
	if len(got) != 2 {
		t.Fatalf("pushes: %+v", got)
	}
	if got[0].Title != "Beleg-Helfer" || got[0].Body != "3 neue Ausschreibungen" || got[0].URL != "/feed" || got[0].Tag != w.helper.ID {
		t.Fatalf("result push: %+v", got[0])
	}
	if got[1].Title != "Beleg-Helfer" || got[1].Body != "Stimmt dieser Beleg?" {
		t.Fatalf("ask push: %+v", got[1])
	}

	// Nothing when the person is in the app anyway: the result after they
	// answered, a run they started by hand, a test run.
	if err := w.card(t, &mail, "result", "Gespeichert", ""); err != nil {
		t.Fatal(err)
	}
	manual := w.run(t, store.TriggerManual, false)
	test := w.run(t, store.TriggerSchedule, true)
	for _, r := range []store.Run{manual, test} {
		if err := w.card(t, &r, "result", "Fertig", ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(w.b.messages()); n != 2 {
		t.Fatalf("%d pushes, want still 2", n)
	}

	// A run that found nothing is in the Feed, but doesn't buzz.
	quiet := store.FeedCard{ID: store.NewID(), WorkspaceID: w.helper.WorkspaceID, UserID: owner, HelperID: &w.helper.ID,
		Type: "result", Title: "Diesmal nichts Neues", Payload: map[string]any{"nothing": true}}
	if err := w.st.CreateFeedCard(ctx, &quiet); err != nil {
		t.Fatal(err)
	}
	if n := len(w.b.messages()); n != 2 {
		t.Fatalf("%d pushes after a quiet run, want still 2", n)
	}

	// A note without a run (an ignored email) is announced too.
	if err := w.card(t, nil, "problem", "E-Mail ignoriert", "…"); err != nil {
		t.Fatal(err)
	}
	if got := w.b.messages(); len(got) != 3 || got[2].Body != "E-Mail ignoriert" {
		t.Fatalf("note push: %+v", got)
	}

	// The same card again (a retried job): refused by the store, no push.
	c := store.FeedCard{ID: store.NewID(), WorkspaceID: w.helper.WorkspaceID, UserID: owner, Type: "problem", Title: "Einmal"}
	if err := w.st.CreateFeedCard(ctx, &c); err != nil {
		t.Fatal(err)
	}
	again := store.FeedCard{ID: c.ID, WorkspaceID: w.helper.WorkspaceID, UserID: owner, Type: "problem", Title: "Einmal"}
	if err := w.st.CreateFeedCard(ctx, &again); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second create: %v", err)
	}
	if n := len(w.b.messages()); n != 4 {
		t.Fatalf("%d pushes, want 4", n)
	}
}

func TestDeadSubscriptionsAreDropped(t *testing.T) {
	w := newWorld(t)
	w.b.status = http.StatusGone
	// The push fails, the card is saved all the same.
	if err := w.card(t, nil, "problem", "Hallo", ""); err != nil {
		t.Fatal(err)
	}
	subs, err := w.mem.PushSubscriptions(context.Background(), owner)
	if err != nil || len(subs) != 0 {
		t.Fatalf("subscriptions left: %+v %v", subs, err)
	}
	// A push service that is down doesn't cost the subscription.
	w2 := newWorld(t)
	w2.b.status = http.StatusServiceUnavailable
	if err := w2.card(t, nil, "problem", "Hallo", ""); err != nil {
		t.Fatal(err)
	}
	if subs, _ := w2.mem.PushSubscriptions(context.Background(), owner); len(subs) != 1 {
		t.Fatalf("subscription dropped on a 503: %+v", subs)
	}
}

func TestWrapWithoutKeys(t *testing.T) {
	mem := memory.New()
	if push.New(mem, "", "", "", nil) != nil {
		t.Fatal("a notifier without keys")
	}
	if st := push.Wrap(mem, nil); st != store.Store(mem) {
		t.Fatal("Wrap(nil) should return the store itself")
	}
}

package do_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/posta/postatest"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type socialEnv struct {
	st    *memory.Store
	fake  *postatest.Fake
	brick do.Social
	rc    *engine.RunContext
	blobs *engine.MemoryBlobs
	place store.Place
}

// A workspace with social posting on (an account Laamo made), Bluesky and
// Instagram connected.
func setupSocial(t *testing.T, own bool) *socialEnv {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	keys, err := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(k), "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fake, svc := postatest.New(t)
	svc.Poll = time.Millisecond
	st := memory.New()
	rc := brickstest.RunContext("do.social")
	blobs := &engine.MemoryBlobs{}
	rc.Blobs = blobs
	chans := []posta.Channel{
		{ID: "11", Platform: "bluesky", Username: "anna.bsky.social", IsActive: true},
		{ID: "12", Platform: "instagram", Username: "annas_bakery", IsActive: true},
		{ID: "13", Platform: "linkedin", Username: "anna", DisplayName: "Anna Krause", IsActive: false, ConnectionError: "expired"},
	}
	var token string
	meta := map[string]any{"source": "issued", "external_user_id": rc.Run.UserID}
	if own {
		token = fake.OwnAccount("anna@example.com", chans...)
		meta = map[string]any{"source": "pasted"}
	} else {
		a, err := svc.Partner.CreateAccount(ctx, posta.NewAccount{Email: "anna@example.com", ExternalUserID: rc.Run.UserID, TermsAcceptedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		token = a.APIToken
		fake.SetChannels(rc.Run.UserID, chans...)
	}
	p := store.Place{ID: store.NewID(), WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, Type: "posta", Label: "Posta", Meta: meta}
	sealed, err := keys.Encrypt([]byte(token), crypto.AAD("places", p.ID, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	p.SecretCiphertext = new(base64.StdEncoding.EncodeToString(sealed))
	if err := st.CreatePlace(ctx, &p); err != nil {
		t.Fatal(err)
	}
	return &socialEnv{st: st, fake: fake, brick: do.Social{Store: st, Keys: keys, Posta: svc}, rc: rc, blobs: blobs, place: p}
}

func (e *socialEnv) run(t *testing.T, in []engine.Card, cfg map[string]any) ([]engine.Card, error) {
	t.Helper()
	if cfg["channels"] == nil {
		cfg["channels"] = []any{"11"}
	}
	return brickstest.Contract(t, "do.social", e.brick, e.rc, in, cfg)
}

func drafts(n int) []engine.Card {
	out := make([]engine.Card, n)
	for i := range out {
		out[i] = engine.Card{ID: fmt.Sprintf("d%d", i), Kind: "draft"}
		out[i].Set("body", fmt.Sprintf("Fresh sourdough, batch %d", i), engine.TrustExtracted)
	}
	return out
}

// picture stores a small JPEG as Draw a picture would and returns its path.
func (e *socialEnv) picture(t *testing.T, name string) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("images/%s/%s/%s.jpg", e.rc.Run.WorkspaceID, e.rc.Run.ID, name)
	if err := e.blobs.Put(context.Background(), path, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	return path, buf.Bytes()
}

func (e *socialEnv) outbound(t *testing.T, cardID string) store.OutboundMessage {
	t.Helper()
	m, err := e.st.GetOutboundByKey(context.Background(), e.rc.Run.ID+":b1:"+cardID)
	if err != nil {
		t.Fatalf("outbound %s: %v", cardID, err)
	}
	return m
}

func TestSocialPostsNow(t *testing.T) {
	e := setupSocial(t, false)
	out, err := e.run(t, drafts(2), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	posts := e.fake.Posts()
	if len(posts) != 2 || posts[0].Caption != "Fresh sourdough, batch 0" || posts[0].Status != "processing" || posts[0].Publish != 1 ||
		strings.Join(posts[0].Channels, ",") != "11" || len(posts[0].MediaIDs) != 0 {
		t.Fatalf("posts: %+v", posts)
	}
	for _, c := range out {
		if c.Text("posted_at") == "" {
			t.Fatalf("card %s has no posted_at", c.ID)
		}
	}
	m := e.outbound(t, "d0")
	if m.Status != "sent" || m.Channel != "posta" || *m.ProviderMessageID != posts[0].ID || *m.Body != posts[0].Caption || m.SentAt == nil {
		t.Fatalf("outbound: %+v", m)
	}

	// The job runs again (the worker died after posting): no second post.
	if _, err := e.run(t, drafts(2), map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if posts := e.fake.Posts(); len(posts) != 2 || posts[0].Publish != 1 {
		t.Fatalf("after a retry: %+v", posts)
	}
}

func TestSocialTestRunPostsNothing(t *testing.T) {
	e := setupSocial(t, false)
	e.rc.TestMode = true
	out, err := e.run(t, drafts(1), map[string]any{})
	if err != nil || len(out) != 1 || out[0].Text("posted_at") != "" {
		t.Fatalf("test run: %+v %v", out, err)
	}
	if len(e.fake.Posts()) != 0 || len(e.fake.Calls) > 1 { // only the account made in setup
		t.Fatalf("a test run talked to Posta: %v", e.fake.Calls)
	}
}

func TestSocialWithAPicture(t *testing.T) {
	e := setupSocial(t, false)
	in := drafts(1)
	path, data := e.picture(t, "d0")
	in[0].Set("image", path, engine.TrustExtracted)
	if _, err := e.run(t, in, map[string]any{"channels": []any{"11", "12"}}); err != nil {
		t.Fatal(err)
	}
	posts := e.fake.Posts()
	if len(posts) != 1 || len(posts[0].MediaIDs) != 1 || strings.Join(posts[0].Channels, ",") != "11,12" {
		t.Fatalf("posts: %+v", posts)
	}
	if !bytes.Equal(e.fake.Picture(posts[0].MediaIDs[0]), data) {
		t.Fatal("the picture at Posta isn't the card's picture")
	}

	// Instagram without a picture: said before anything is made.
	e2 := setupSocial(t, false)
	_, err := e2.run(t, drafts(1), map[string]any{"channels": []any{"12"}})
	if !errors.Is(err, queue.ErrPermanent) || !strings.Contains(err.Error(), "Instagram takes no post without a picture") || len(e2.fake.Posts()) != 0 {
		t.Fatalf("no picture: %v, %d posts", err, len(e2.fake.Posts()))
	}
	// A path into another workspace's pictures is never read.
	e3 := setupSocial(t, false)
	in = drafts(1)
	other := "images/" + store.NewID() + "/run/x.jpg"
	_ = e3.blobs.Put(context.Background(), other, data)
	in[0].Set("image", other, engine.TrustExtracted)
	if _, err := e3.run(t, in, map[string]any{}); !errors.Is(err, queue.ErrPermanent) || len(e3.fake.Posts()) != 0 {
		t.Fatalf("foreign picture: %v", err)
	}
}

func TestSocialAtATime(t *testing.T) {
	e := setupSocial(t, false)
	// The test clock: 10 November 2026, 08:00 UTC = 09:00 in Berlin.
	if _, err := e.run(t, drafts(1), map[string]any{"when": "at", "time": "18:30"}); err != nil {
		t.Fatal(err)
	}
	p := e.fake.Posts()[0]
	if p.Status != "scheduled" || p.Publish != 0 || p.At == nil || !p.At.Equal(time.Date(2026, 11, 10, 17, 30, 0, 0, time.UTC)) {
		t.Fatalf("scheduled: %+v", p)
	}
	if m := e.outbound(t, "d0"); m.Status != "sent" || m.ScheduledFor == nil {
		t.Fatalf("outbound: %+v", m)
	}
	// A time that has passed today means tomorrow.
	e2 := setupSocial(t, false)
	if _, err := e2.run(t, drafts(1), map[string]any{"when": "at", "time": "07:00"}); err != nil {
		t.Fatal(err)
	}
	if p := e2.fake.Posts()[0]; !p.At.Equal(time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("tomorrow: %v", p.At)
	}
}

func TestSocialDraft(t *testing.T) {
	// With your own Posta account, a draft waits for you in Posta.
	e := setupSocial(t, true)
	if _, err := e.run(t, drafts(1), map[string]any{"when": "draft"}); err != nil {
		t.Fatal(err)
	}
	if p := e.fake.Posts()[0]; p.Status != "draft" || p.Publish != 0 {
		t.Fatalf("draft: %+v", p)
	}
	if m := e.outbound(t, "d0"); m.Status != "draft" {
		t.Fatalf("outbound: %+v", m)
	}
	// An account Laamo made has nowhere to open a draft: it goes out.
	e2 := setupSocial(t, false)
	if _, err := e2.run(t, drafts(1), map[string]any{"when": "draft"}); err != nil {
		t.Fatal(err)
	}
	if p := e2.fake.Posts()[0]; p.Status != "processing" {
		t.Fatalf("draft on a Laamo-made account: %+v", p)
	}
}

func TestSocialOnlyTheFirstCard(t *testing.T) {
	e := setupSocial(t, false)
	out, err := e.run(t, drafts(3), map[string]any{"mode": "one"})
	if err != nil || len(out) != 3 || len(e.fake.Posts()) != 1 || out[1].Text("posted_at") != "" {
		t.Fatalf("one: %d posts, %v", len(e.fake.Posts()), err)
	}
	// Never more than MaxPosts in a run.
	e2 := setupSocial(t, false)
	if _, err := e2.run(t, drafts(do.MaxPosts+3), map[string]any{}); err != nil || len(e2.fake.Posts()) != do.MaxPosts {
		t.Fatalf("limit: %d posts, %v", len(e2.fake.Posts()), err)
	}
}

func TestSocialNeverPostsTwice(t *testing.T) {
	// The post was made but the answer got lost on the way, and the same
	// when Posta answers with a server error: nobody knows whether it is
	// out there. The retry doesn't make a second one; it stops and says so.
	for name, lose := range map[string]func(f *postatest.Fake){
		"the answer got lost": func(f *postatest.Fake) { f.LoseCreate = 1 },
		"a server error":      func(f *postatest.Fake) { f.FailCreate = 1 },
	} {
		e := setupSocial(t, false)
		lose(e.fake)
		if _, err := e.run(t, drafts(1), map[string]any{}); err == nil || errors.Is(err, queue.ErrPermanent) {
			t.Fatalf("%s: the first attempt should fail and be retried: %v", name, err)
		}
		if m := e.outbound(t, "d0"); m.Status != "pending" {
			t.Fatalf("%s: %+v", name, m)
		}
		before := len(e.fake.Posts())
		_, err := e.run(t, drafts(1), map[string]any{})
		if !errors.Is(err, queue.ErrPermanent) || !strings.Contains(err.Error(), "may or may not have gone out") {
			t.Fatalf("%s: the retry: %v", name, err)
		}
		if n := len(e.fake.Posts()); n != before || n > 1 {
			t.Fatalf("%s: %d posts after the retry (%d before)", name, n, before)
		}
	}

	// The post was made and we know its id, but the worker died before it
	// was sent: the retry sends that same post, once.
	e := setupSocial(t, false)
	ctx := context.Background()
	user := e.brick.Posta.As(e.fake.Token(e.rc.Run.UserID))
	made, err := user.CreatePost(ctx, posta.NewPost{Caption: "Fresh sourdough, batch 0", Channels: []string{"11"}})
	if err != nil {
		t.Fatal(err)
	}
	m := store.OutboundMessage{WorkspaceID: e.rc.Run.WorkspaceID, UserID: e.rc.Run.UserID, RunID: &e.rc.Run.ID,
		IdempotencyKey: e.rc.Run.ID + ":b1:d0", Channel: "posta", Recipient: "place:" + e.place.ID, ProviderMessageID: &made.ID}
	if err := e.st.CreateOutbound(ctx, &m); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, err := e.run(t, drafts(1), map[string]any{})
		if err != nil || out[0].Text("posted_at") == "" {
			t.Fatalf("resume: %v", err)
		}
	}
	if posts := e.fake.Posts(); len(posts) != 1 || posts[0].Publish != 1 || posts[0].Status != "processing" {
		t.Fatalf("resume: %+v", posts)
	}
}

func TestSocialProblems(t *testing.T) {
	for name, tc := range map[string]struct {
		prep func(e *socialEnv)
		cfg  map[string]any
		want string
	}{
		"free days over":                     {func(e *socialEnv) { e.fake.EndTrial(e.rc.Run.UserID) }, nil, "5 free days of social posting are over"},
		"an account that was disconnected":   {nil, map[string]any{"channels": []any{"99"}}, "isn't connected anymore"},
		"an account that needs reconnecting": {nil, map[string]any{"channels": []any{"13"}}, "Anna Krause (Linkedin) needs reconnecting"},
		"social posting switched off":        {func(e *socialEnv) { _ = e.st.DeletePlace(context.Background(), e.place.ID) }, nil, "Social posting is off"},
	} {
		e := setupSocial(t, false)
		if tc.prep != nil {
			tc.prep(e)
		}
		cfg := tc.cfg
		if cfg == nil {
			cfg = map[string]any{}
		}
		_, err := e.run(t, drafts(1), cfg)
		if !errors.Is(err, queue.ErrPermanent) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
		if n := len(e.fake.Posts()); n != 0 {
			t.Errorf("%s: %d posts", name, n)
		}
	}
	// In German for a German workspace.
	e := setupSocial(t, false)
	e.rc.Workspace.Locale = "de"
	e.fake.EndTrial(e.rc.Run.UserID)
	if _, err := e.run(t, drafts(1), map[string]any{}); err == nil || !strings.Contains(err.Error(), "kostenlosen Tage") {
		t.Fatalf("German: %v", err)
	}
}

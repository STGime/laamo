package api_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/posta/postatest"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

type socialEnv struct {
	env
	fake *postatest.Fake
	keys *crypto.Keyring
	ws   store.Workspace // Anna's; Ben is a member
	ws2  store.Workspace // Anna's second
}

func newSocialEnv(t *testing.T) socialEnv {
	t.Helper()
	st := memory.New()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	keys, err := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(key), "")
	if err != nil {
		t.Fatal(err)
	}
	fake, svc := postatest.New(t)
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}, Keys: keys, Posta: svc}).Handler())
	t.Cleanup(srv.Close)
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	ws2 := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Café", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	st.PutMember(store.Member{WorkspaceID: ws2.ID, UserID: anna.ID, Role: store.RoleOwner})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: ben.ID, Role: store.RoleMember})
	return socialEnv{env: env{url: srv.URL, store: st}, fake: fake, keys: keys, ws: ws, ws2: ws2}
}

func yes() *bool { return new(true) }

func TestSocialWithANewPostaAccount(t *testing.T) {
	e := newSocialEnv(t)
	ctx := context.Background()
	asAnna, asBen := e.client(t, "anna"), e.client(t, "ben")
	id := wsID(e.ws.ID)

	if r := must(asAnna.GetSocialWithResponse(ctx, id)); r.StatusCode() != 200 || r.JSON200.State != api.SocialPostingStateOff {
		t.Fatalf("before: %d %s", r.StatusCode(), r.Body)
	}
	// No account without the consent, and none for a member.
	if r := must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{})); r.StatusCode() != 400 {
		t.Fatalf("no consent: %d %s", r.StatusCode(), r.Body)
	}
	if r := must(asBen.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{AcceptTerms: yes()})); r.StatusCode() != 403 {
		t.Fatalf("member: %d", r.StatusCode())
	}
	if len(e.fake.Calls) != 0 {
		t.Fatalf("Posta was called: %v", e.fake.Calls)
	}

	r := must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{AcceptTerms: yes()}))
	if r.StatusCode() != 200 || r.JSON200.State != api.SocialPostingStateOn || *r.JSON200.Source != api.Issued {
		t.Fatalf("enable: %d %s", r.StatusCode(), r.Body)
	}
	if p := r.JSON200.Plan; p == nil || p.Type != "trial" || !p.Active || *p.DaysLeft != 5 || *p.MaxChannels != 10 || *r.JSON200.Channels != 0 {
		t.Fatalf("plan: %s", r.Body)
	}
	// The token is stored encrypted, under Anna's Laamo user id.
	token := e.fake.Token(anna.ID)
	ps, _ := e.store.ListPlaces(ctx, e.ws.ID)
	if len(ps) != 1 || ps[0].Type != "posta" || ps[0].Meta["external_user_id"] != anna.ID || ps[0].Meta["terms_accepted_at"] == nil {
		t.Fatalf("place: %+v", ps)
	}
	env, _ := base64.StdEncoding.DecodeString(*ps[0].SecretCiphertext)
	if plain, err := e.keys.Decrypt(env, crypto.AAD("places", ps[0].ID, "secret")); err != nil || string(plain) != token || token == "" {
		t.Fatalf("stored token: %q %v", plain, err)
	}
	if strings.Contains(string(r.Body), token) {
		t.Fatal("the token went to the app")
	}

	// Not among the chats.
	if l := must(asAnna.ListPlacesWithResponse(ctx, id)); len(l.JSON200.Places) != 0 {
		t.Fatalf("chats: %s", l.Body)
	}
	// Switching on again leaves the account and its token alone.
	if r := must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{AcceptTerms: yes()})); r.StatusCode() != 200 || e.fake.Token(anna.ID) != token {
		t.Fatalf("again: %d, token changed: %v", r.StatusCode(), e.fake.Token(anna.ID) != token)
	}
	// Ben, a member, sees that it is on.
	if r := must(asBen.GetSocialWithResponse(ctx, id)); r.JSON200.State != api.SocialPostingStateOn {
		t.Fatalf("member sees: %s", r.Body)
	}
	// One account per person: not in Anna's second workspace as well.
	id2 := wsID(e.ws2.ID)
	if r := must(asAnna.GetSocialWithResponse(ctx, id2)); r.JSON200.State != api.SocialPostingStateElsewhere || *r.JSON200.Elsewhere != "Bäckerei" {
		t.Fatalf("second workspace: %s", r.Body)
	}
	if r := must(asAnna.EnableSocialWithResponse(ctx, id2, api.EnableSocialJSONRequestBody{AcceptTerms: yes()})); r.StatusCode() != 409 || r.JSON409.Code != api.ErrorCodePostaElsewhere {
		t.Fatalf("second workspace enable: %d %s", r.StatusCode(), r.Body)
	}

	// Off: Posta is down, so nothing is removed here either.
	if r := must(asBen.DisableSocialWithResponse(ctx, id)); r.StatusCode() != 403 {
		t.Fatalf("member disables: %d", r.StatusCode())
	}
	e.fake.Down = true
	if r := must(asAnna.DisableSocialWithResponse(ctx, id)); r.StatusCode() != 503 {
		t.Fatalf("disable while Posta is down: %d", r.StatusCode())
	}
	if ps, _ := e.store.ListPlaces(ctx, e.ws.ID); len(ps) != 1 {
		t.Fatal("the account was forgotten here but still exists at Posta")
	}
	e.fake.Down = false
	if r := must(asAnna.DisableSocialWithResponse(ctx, id)); r.StatusCode() != 204 {
		t.Fatalf("disable: %d %s", r.StatusCode(), r.Body)
	}
	if ps, _ := e.store.ListPlaces(ctx, e.ws.ID); len(ps) != 0 || e.fake.Token(anna.ID) != "" {
		t.Fatalf("after disable: %d places, Posta account left: %v", len(ps), e.fake.Token(anna.ID) != "")
	}
	if r := must(asAnna.DisableSocialWithResponse(ctx, id)); r.StatusCode() != 204 {
		t.Fatalf("disable twice: %d", r.StatusCode())
	}
}

func TestSocialWithYourOwnPostaAccount(t *testing.T) {
	e := newSocialEnv(t)
	ctx := context.Background()
	asAnna := e.client(t, "anna")
	id := wsID(e.ws.ID)
	key := e.fake.OwnAccount(anna.Email, posta.Channel{ID: "1", Platform: "instagram", Username: "annas_bakery", IsActive: true})

	// Her email has a Posta account already: Laamo can't make one.
	r := must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{AcceptTerms: yes()}))
	if r.StatusCode() != 409 || r.JSON409.Code != api.ErrorCodePostaAccountExists {
		t.Fatalf("existing email: %d %s", r.StatusCode(), r.Body)
	}
	if r := must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{ApiKey: new("posta_wrong")})); r.StatusCode() != 400 || r.JSON400.Code != api.ErrorCodePostaKeyInvalid {
		t.Fatalf("wrong key: %d %s", r.StatusCode(), r.Body)
	}
	if ps, _ := e.store.ListPlaces(ctx, e.ws.ID); len(ps) != 0 {
		t.Fatalf("saved: %+v", ps)
	}

	r = must(asAnna.EnableSocialWithResponse(ctx, id, api.EnableSocialJSONRequestBody{ApiKey: new("  " + key + " ")}))
	if r.StatusCode() != 200 || *r.JSON200.Source != api.Pasted || *r.JSON200.Channels != 1 || r.JSON200.Plan != nil {
		t.Fatalf("paste: %d %s", r.StatusCode(), r.Body)
	}
	// Switching off only forgets the key: the account is hers, and the
	// partner API is never asked about it.
	e.fake.Calls = nil
	if r := must(asAnna.DisableSocialWithResponse(ctx, id)); r.StatusCode() != 204 {
		t.Fatalf("disable: %d", r.StatusCode())
	}
	if len(e.fake.Calls) != 0 {
		t.Fatalf("Posta was called for her own account: %v", e.fake.Calls)
	}
	if _, err := (&posta.User{Token: key, URL: e.fake.URL}).Channels(ctx); err != nil {
		t.Fatalf("her own account stopped working: %v", err)
	}
}

func TestSocialOffWithoutPartnerKey(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}}).Handler())
	t.Cleanup(srv.Close)
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := env{url: srv.URL, store: st}.client(t, "anna")
	ctx := context.Background()
	if r := must(c.GetSocialWithResponse(ctx, wsID(ws.ID))); r.JSON200.State != api.SocialPostingStateUnavailable {
		t.Fatalf("state: %s", r.Body)
	}
	if r := must(c.EnableSocialWithResponse(ctx, wsID(ws.ID), api.EnableSocialJSONRequestBody{AcceptTerms: yes()})); r.StatusCode() != 503 {
		t.Fatalf("enable: %d", r.StatusCode())
	}
}

package api_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// fakeChat records test messages; a webhook containing "gone" was deleted.
type fakeChat struct{ sent []string }

func (f *fakeChat) Send(_ context.Context, _, webhook string, m chat.Message) error {
	if strings.Contains(webhook, "gone") {
		return chat.ErrGone
	}
	f.sent = append(f.sent, webhook+" "+m.Text)
	return nil
}

func TestPlaces(t *testing.T) {
	st := memory.New()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	keys, err := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(key), "")
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeChat{}
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}, Keys: keys, Chat: fc}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: ben.ID, Role: store.RoleMember})
	anna, ben := e.client(t, "anna"), e.client(t, "ben")
	hook := "https://discord.com/api/webhooks/123/secret-token-abcd"

	// A link that isn't a webhook, or one Discord deleted: nothing is saved.
	for url, code := range map[string]api.ErrorCode{"https://example.com/x": api.ErrorCodeWebhookInvalid,
		"https://discord.com/api/webhooks/1/gone": api.ErrorCodeWebhookGone} {
		r := must(anna.CreatePlaceWithResponse(ctx, wsID(ws.ID), api.NewPlace{Type: "discord", Label: "Team", WebhookUrl: url}))
		if r.StatusCode() != 400 || r.JSON400.Code != code {
			t.Fatalf("%s: %d %s", url, r.StatusCode(), r.Body)
		}
	}
	if ps, _ := st.ListPlaces(ctx, ws.ID); len(ps) != 0 {
		t.Fatalf("saved %d places", len(ps))
	}
	if r := must(ben.CreatePlaceWithResponse(ctx, wsID(ws.ID), api.NewPlace{Type: "discord", Label: "Team", WebhookUrl: hook})); r.StatusCode() != 403 {
		t.Fatalf("member connects: %d", r.StatusCode())
	}

	// Connect: test message in German, saved encrypted, URL never returned.
	r := must(anna.CreatePlaceWithResponse(ctx, wsID(ws.ID), api.NewPlace{Type: "discord", Label: " Team ", WebhookUrl: hook}))
	if r.StatusCode() != 201 || r.JSON201.Label != "Team" || r.JSON201.VerifiedAt == nil || *r.JSON201.Last4 != "abcd" {
		t.Fatalf("connect: %d %s", r.StatusCode(), r.Body)
	}
	if len(fc.sent) != 1 || !strings.Contains(fc.sent[0], "Laamo ist verbunden") || strings.Contains(string(r.Body), "secret-token") {
		t.Fatalf("sent %v, body %s", fc.sent, r.Body)
	}
	p, _ := st.GetPlace(ctx, r.JSON201.Id.String())
	if p.SecretCiphertext == nil || strings.Contains(*p.SecretCiphertext, "secret-token") {
		t.Fatal("webhook not encrypted")
	}

	list := must(ben.ListPlacesWithResponse(ctx, wsID(ws.ID)))
	if list.StatusCode() != 200 || len(list.JSON200.Places) != 1 || strings.Contains(string(list.Body), "secret-token") {
		t.Fatalf("list: %d %s", list.StatusCode(), list.Body)
	}
	if tr := must(ben.TestPlaceWithResponse(ctx, r.JSON201.Id)); tr.StatusCode() != 200 || len(fc.sent) != 2 || !strings.HasPrefix(fc.sent[1], hook) {
		t.Fatalf("test again: %d %v", tr.StatusCode(), fc.sent)
	}
	if d := must(ben.DeletePlaceWithResponse(ctx, r.JSON201.Id)); d.StatusCode() != 403 {
		t.Fatalf("member deletes: %d", d.StatusCode())
	}
	if d := must(anna.DeletePlaceWithResponse(ctx, r.JSON201.Id)); d.StatusCode() != 204 {
		t.Fatalf("delete: %d", d.StatusCode())
	}
	if l := must(anna.ListPlacesWithResponse(ctx, wsID(ws.ID))); len(l.JSON200.Places) != 0 {
		t.Fatalf("after delete: %s", l.Body)
	}
}

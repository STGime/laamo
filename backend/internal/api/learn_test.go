package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// The helper page shows how many swipes a helper learned from, and Forget
// clears them (#76).
func TestForgetLearned(t *testing.T) {
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "en", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")
	h := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "morning-briefing",
		Answers: map[string]string{"chat": "feed", "business": "Bakery", "city": "Leipzig"}})).JSON201.Helper

	if got := must(c.GetHelperWithResponse(ctx, h.Id)); got.JSON200.Learned != 0 {
		t.Fatalf("new helper learned %d", got.JSON200.Learned)
	}
	_ = st.AddPickExamples(ctx, []store.PickExample{
		{HelperID: h.Id.String(), StepID: store.NewID(), CardID: "a", Kept: true},
		{HelperID: h.Id.String(), StepID: store.NewID(), CardID: "b"},
	})
	if got := must(c.GetHelperWithResponse(ctx, h.Id)); got.JSON200.Learned != 2 {
		t.Fatalf("learned %d", got.JSON200.Learned)
	}
	// Someone else can't make it forget.
	if r := must(e.client(t, "ben").ForgetLearnedWithResponse(ctx, h.Id)); r.StatusCode() != 404 {
		t.Fatalf("ben: %d", r.StatusCode())
	}
	if r := must(c.ForgetLearnedWithResponse(ctx, h.Id)); r.StatusCode() != 204 {
		t.Fatalf("forget: %d", r.StatusCode())
	}
	if got := must(c.GetHelperWithResponse(ctx, h.Id)); got.JSON200.Learned != 0 {
		t.Fatalf("after Forget: %d", got.JSON200.Learned)
	}
}

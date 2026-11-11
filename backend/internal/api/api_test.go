package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/auth"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// fakeAuth maps tokens to users.
type fakeAuth map[string]auth.User

func (f fakeAuth) Verify(_ context.Context, token string) (auth.User, error) {
	if u, ok := f[token]; ok {
		return u, nil
	}
	return auth.User{}, auth.ErrInvalidToken
}

var (
	anna = auth.User{ID: "aaaaaaaa-0000-0000-0000-000000000001", Email: "anna@example.com"}
	ben  = auth.User{ID: "bbbbbbbb-0000-0000-0000-000000000002", Email: "ben@example.com"}
)

type env struct {
	url   string
	store *memory.Store
}

func newEnv(t *testing.T, cfg config.Config) env {
	t.Helper()
	st := memory.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{
		Config: cfg, Store: st, Auth: fakeAuth{"anna": anna, "ben": ben},
	}).Handler())
	t.Cleanup(srv.Close)
	return env{url: srv.URL, store: st}
}

// client returns the generated client, signed in with token ("" = no token).
func (e env) client(t *testing.T, token string) *api.ClientWithResponses {
	t.Helper()
	opts := []api.ClientOption{}
	if token != "" {
		opts = append(opts, api.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
			r.Header.Set("Authorization", "Bearer "+token)
			return nil
		}))
	}
	c, err := api.NewClientWithResponses(e.url+api.BaseURL, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// must unwraps a client call; a transport error means the test setup is
// broken, so it panics (which fails the test).
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Health round-trips through the generated client and server, without a token.
func TestHealthRoundTrip(t *testing.T) {
	eurobase := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(eurobase.Close)

	cases := []struct {
		name string
		cfg  config.Config
		want api.HealthEurobase
	}{
		{"not configured", config.Config{Env: "dev"}, api.HealthEurobaseNotConfigured},
		{"reachable", config.Config{Env: "dev", EurobaseURL: eurobase.URL, EurobaseSecretKey: "eb_sk_x"}, api.HealthEurobaseReachable},
		{"unreachable", config.Config{Env: "dev", EurobaseURL: "http://127.0.0.1:1", EurobaseSecretKey: "eb_sk_x"}, api.HealthEurobaseUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.cfg)
			res := must(e.client(t, "").GetHealthWithResponse(context.Background()))
			if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
				t.Fatalf("status %d: %s", res.StatusCode(), res.Body)
			}
			if h := res.JSON200; h.Status != "ok" || h.Env != "dev" || h.Eurobase != tc.want {
				t.Fatalf("health: %+v", h)
			}
		})
	}
}

func TestAuthRequired(t *testing.T) {
	e := newEnv(t, config.Config{})
	ctx := context.Background()
	for _, token := range []string{"", "wrong"} {
		res := must(e.client(t, token).GetMeWithResponse(ctx))
		if res.StatusCode() != http.StatusUnauthorized || res.JSON401 == nil || res.JSON401.Code != api.ErrorCodeUnauthorized {
			t.Fatalf("token %q: %d %s", token, res.StatusCode(), res.Body)
		}
	}
}

func TestWorkspaceFlow(t *testing.T) {
	e := newEnv(t, config.Config{})
	ctx := context.Background()
	asAnna, asBen := e.client(t, "anna"), e.client(t, "ben")

	// A new user has no workspaces yet.
	me := must(asAnna.GetMeWithResponse(ctx))
	if me.StatusCode() != 200 || me.JSON200.User.Email != anna.Email || len(me.JSON200.Workspaces) != 0 {
		t.Fatalf("me: %d %s", me.StatusCode(), me.Body)
	}

	// Create one (onboarding).
	tz, de := "Europe/Vienna", api.De
	created := must(asAnna.CreateWorkspaceWithResponse(ctx, api.NewWorkspace{Name: "Bäckerei Krause", Timezone: &tz, Locale: &de}))
	if created.StatusCode() != http.StatusCreated {
		t.Fatalf("create: %d %s", created.StatusCode(), created.Body)
	}
	ws := created.JSON201
	if ws.Role != api.Owner || ws.Timezone != tz || ws.Locale != api.De || ws.Plan != api.Free || ws.Profile == nil {
		t.Fatalf("created: %+v", ws)
	}

	me = must(asAnna.GetMeWithResponse(ctx))
	if len(me.JSON200.Workspaces) != 1 || me.JSON200.Workspaces[0].Id != ws.Id {
		t.Fatalf("me after create: %s", me.Body)
	}

	got := must(asAnna.GetWorkspaceWithResponse(ctx, ws.Id))
	if got.StatusCode() != 200 || got.JSON200.Name != "Bäckerei Krause" {
		t.Fatalf("get: %d %s", got.StatusCode(), got.Body)
	}

	// Someone else's workspace looks like it doesn't exist.
	if res := must(asBen.GetWorkspaceWithResponse(ctx, ws.Id)); res.StatusCode() != http.StatusNotFound || res.JSON404.Code != api.ErrorCodeNotFound {
		t.Fatalf("other user get: %d", res.StatusCode())
	}
	if res := must(asBen.UpdateWorkspaceWithResponse(ctx, ws.Id, api.WorkspaceUpdate{Name: new("mine")})); res.StatusCode() != http.StatusNotFound {
		t.Fatalf("other user update: %d", res.StatusCode())
	}

	// About me: set, then forget one fact.
	upd := must(asAnna.UpdateWorkspaceWithResponse(ctx, ws.Id, api.WorkspaceUpdate{
		Profile: &api.Profile{"city": "Wien", "business": "bakery"},
	}))
	if upd.StatusCode() != 200 || upd.JSON200.Profile["city"] != "Wien" {
		t.Fatalf("profile: %d %s", upd.StatusCode(), upd.Body)
	}
	upd = must(asAnna.UpdateWorkspaceWithResponse(ctx, ws.Id, api.WorkspaceUpdate{
		Name: new("  Krause & Söhne "), Profile: &api.Profile{"city": ""},
	}))
	if upd.StatusCode() != 200 || upd.JSON200.Name != "Krause & Söhne" {
		t.Fatalf("rename: %d %s", upd.StatusCode(), upd.Body)
	}
	if _, has := upd.JSON200.Profile["city"]; has || upd.JSON200.Profile["business"] != "bakery" {
		t.Fatalf("forget city, keep business: %v", upd.JSON200.Profile)
	}

	// Bad input → 400 with a reason.
	for name, body := range map[string]api.WorkspaceUpdate{
		"unknown fact": {Profile: &api.Profile{"shoe_size": "42"}},
		"long fact":    {Profile: &api.Profile{"offer": strings.Repeat("x", 501)}},
		"timezone":     {Timezone: new("Mars/Olympus")},
		"empty name":   {Name: new("   ")},
		"locale":       {Locale: new(api.Locale("fr"))},
	} {
		res := must(asAnna.UpdateWorkspaceWithResponse(ctx, ws.Id, body))
		if res.StatusCode() != http.StatusBadRequest || res.JSON400 == nil || res.JSON400.Error == "" {
			t.Fatalf("%s: %d %s", name, res.StatusCode(), res.Body)
		}
	}

	// A plain member can read but not change.
	e.store.PutMember(store.Member{WorkspaceID: ws.Id.String(), UserID: ben.ID, Role: store.RoleMember})
	if res := must(asBen.GetWorkspaceWithResponse(ctx, ws.Id)); res.StatusCode() != 200 || res.JSON200.Role != api.Member {
		t.Fatalf("member get: %d %s", res.StatusCode(), res.Body)
	}
	if res := must(asBen.UpdateWorkspaceWithResponse(ctx, ws.Id, api.WorkspaceUpdate{Name: new("x")})); res.StatusCode() != http.StatusForbidden {
		t.Fatalf("member update: %d", res.StatusCode())
	}
}

func TestCreateWorkspaceValidation(t *testing.T) {
	e := newEnv(t, config.Config{})
	ctx := context.Background()
	c := e.client(t, "anna")

	bad := "Mars/Olympus"
	res := must(c.CreateWorkspaceWithResponse(ctx, api.NewWorkspace{Name: "x", Timezone: &bad}))
	if res.StatusCode() != http.StatusBadRequest || res.JSON400.Error != "unknown timezone Mars/Olympus" {
		t.Fatalf("bad timezone: %d %s", res.StatusCode(), res.Body)
	}
	for range 5 {
		if res := must(c.CreateWorkspaceWithResponse(ctx, api.NewWorkspace{Name: "w"})); res.StatusCode() != http.StatusCreated {
			t.Fatalf("create: %d", res.StatusCode())
		}
	}
	res = must(c.CreateWorkspaceWithResponse(ctx, api.NewWorkspace{Name: "sixth"}))
	if res.StatusCode() != http.StatusBadRequest || !strings.Contains(res.JSON400.Error, "5 workspaces") {
		t.Fatalf("sixth: %d %s", res.StatusCode(), res.Body)
	}

	// Malformed requests: not JSON, not a uuid.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.url+"/api/workspaces", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer anna")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad json: %d", resp.StatusCode)
	}
	req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, e.url+"/api/workspaces/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer anna")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad uuid: %d", resp.StatusCode)
	}
}

// A store failure is a 500 with the generic body, never a leaked error.
func TestInternalErrorsAreHidden(t *testing.T) {
	st := failingStore{Store: memory.New()}
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}}).Handler())
	t.Cleanup(srv.Close)
	c, _ := api.NewClientWithResponses(srv.URL+api.BaseURL, api.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer anna")
		return nil
	}))
	res := must(c.GetMeWithResponse(context.Background()))
	if res.StatusCode() != http.StatusInternalServerError || strings.Contains(string(res.Body), "secret db detail") {
		t.Fatalf("got %d %s", res.StatusCode(), res.Body)
	}
}

type failingStore struct{ store.Store }

func (failingStore) Memberships(context.Context, string) ([]store.Member, error) {
	return nil, errors.New("secret db detail")
}

func TestSpecIsServed(t *testing.T) {
	e := newEnv(t, config.Config{})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, e.url+"/api/openapi.yaml", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "openapi: 3.0.3") || !strings.Contains(string(body), "/me:") {
		t.Fatalf("spec: %d %.80s", resp.StatusCode, body)
	}
}

// Onboarding sends About me facts and needs with the new workspace.
func TestCreateWorkspaceWithOnboarding(t *testing.T) {
	e := newEnv(t, config.Config{})
	ctx := context.Background()
	c := e.client(t, "anna")
	needs := api.Needs{"find_customers", "stay_up_to_date"}
	res := must(c.CreateWorkspaceWithResponse(ctx, api.NewWorkspace{Name: "Bäckerei Krause", Locale: new(api.Locale("de")),
		Profile: &api.Profile{"name": "Anna", "business": "Bäckerei"}, Needs: &needs}))
	if res.StatusCode() != 201 || res.JSON201.Profile["name"] != "Anna" || len(*res.JSON201.Needs) != 2 {
		t.Fatalf("%d %s", res.StatusCode(), res.Body)
	}
	for _, bad := range []api.NewWorkspace{
		{Name: "X", Needs: &api.Needs{"find_customers", "find_customers"}},
		{Name: "X", Needs: &api.Needs{"find_customers", "sort_emails", "book_meetings", "plan_events"}},
		{Name: "X", Needs: &api.Needs{"world_domination"}},
		{Name: "X", Profile: &api.Profile{"favourite_colour": "red"}},
	} {
		if r := must(c.CreateWorkspaceWithResponse(ctx, bad)); r.StatusCode() != 400 {
			t.Errorf("%+v: %d %s", bad, r.StatusCode(), r.Body)
		}
	}
	one := api.Needs{"plan_events"}
	up := must(c.UpdateWorkspaceWithResponse(ctx, res.JSON201.Id, api.WorkspaceUpdate{Needs: &one}))
	if up.StatusCode() != 200 || len(*up.JSON200.Needs) != 1 || (*up.JSON200.Needs)[0] != "plan_events" || up.JSON200.Profile["business"] != "Bäckerei" {
		t.Fatalf("%d %s", up.StatusCode(), up.Body)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
	qmem "github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

func TestShelfToFirstRun(t *testing.T) {
	st := memory.New()
	q := qmem.New()
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: config.Config{}, Store: st,
		Auth: fakeAuth{"anna": anna, "ben": ben}, Queue: q}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei Krause", Locale: "de", Timezone: "Europe/Berlin",
		Profile: map[string]any{"business": "Bäckerei"}, Needs: []string{"stay_up_to_date"}})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	c := e.client(t, "anna")

	list := must(c.ListTemplatesWithResponse(ctx, wsID(ws.ID)))
	if list.StatusCode() != 200 || len(list.JSON200.Templates) < 2 {
		t.Fatalf("%d %s", list.StatusCode(), list.Body)
	}
	mb := list.JSON200.Templates[0]
	if mb.Slug != "morning-briefing" || mb.Name != "Morgen-Briefing" || mb.Match == 0 || !mb.Brain {
		t.Fatalf("first template %+v", mb)
	}
	if !strings.Contains(mb.Bricks[1].Sentence, "Bäckerei {my city} Neuigkeiten") || len(mb.Questions) != 2 || mb.Questions[0].Key != "city" {
		t.Fatalf("preview %+v", mb)
	}

	bad := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "morning-briefing", Answers: map[string]string{"chat": "feed"}}))
	if bad.StatusCode() != 400 {
		t.Fatalf("missing city: %d", bad.StatusCode())
	}
	res := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "morning-briefing",
		Answers: map[string]string{"city": "Leipzig", "chat": "feed"}, RunTest: new(true)}))
	if res.StatusCode() != 201 || res.JSON201.Helper.Status != "active" || res.JSON201.RunId == nil || *res.JSON201.Helper.Avatar != "📰" {
		t.Fatalf("%d %s", res.StatusCode(), res.Body)
	}

	// Ask once: the city is now an About me fact for every helper.
	w, _ := st.GetWorkspace(ctx, ws.ID)
	if w.Profile["city"] != "Leipzig" {
		t.Fatalf("profile %v", w.Profile)
	}
	// The helper has its stack (no Post to chat), a schedule, and a queued test run.
	h, _ := st.GetHelper(ctx, res.JSON201.Helper.Id.String())
	v, _ := st.GetHelperVersion(ctx, *h.CurrentVersionID)
	if strings.Contains(string(v.Stack), "do.chat") {
		t.Fatalf("stack %s", v.Stack)
	}
	trs, _ := st.HelperTriggers(ctx, h.ID)
	if len(trs) != 1 || trs[0].NextRunAt == nil {
		t.Fatalf("triggers %+v", trs)
	}
	r, err := st.GetRun(ctx, res.JSON201.RunId.String())
	if err != nil || r.TriggerType != "test" || !r.TestMode {
		t.Fatalf("run %+v %v", r, err)
	}
	jobs := q.Jobs()
	var payload map[string]string
	_ = json.Unmarshal(jobs[0].Payload, &payload)
	if len(jobs) != 1 || jobs[0].Kind != "run" || payload["run_id"] != r.ID {
		t.Fatalf("jobs %+v", jobs)
	}

	// Run now, the helper list, and nothing for strangers.
	run := must(c.StartRunWithResponse(ctx, wsID(h.ID), api.StartRunJSONRequestBody{}))
	if run.StatusCode() != 202 || len(q.Jobs()) != 2 {
		t.Fatalf("run now: %d %s", run.StatusCode(), run.Body)
	}
	hs := must(c.ListHelpersWithResponse(ctx, wsID(ws.ID)))
	if hs.StatusCode() != 200 || len(hs.JSON200.Helpers) != 1 || hs.JSON200.Helpers[0].Name != "Morgen-Briefing" {
		t.Fatalf("helpers %s", hs.Body)
	}
	// Two runs so far: the test run and Run now.
	_ = run
	// Manage it: details, a second copy, rename, pause, delete.
	d := must(c.GetHelperWithResponse(ctx, wsID(h.ID)))
	if d.StatusCode() != 200 || len(d.JSON200.Bricks) != 4 || len(d.JSON200.Runs) != 2 || d.JSON200.Helper.NextRunAt == nil ||
		!strings.Contains(d.JSON200.Bricks[1].Sentence, "Bäckerei Leipzig Neuigkeiten") {
		t.Fatalf("detail %d %s", d.StatusCode(), d.Body)
	}
	two := must(c.CreateHelperWithResponse(ctx, wsID(ws.ID), api.NewHelper{Template: "morning-briefing", Answers: map[string]string{"chat": "feed"}}))
	if two.StatusCode() != 201 || two.JSON201.Helper.Name != "Morgen-Briefing (2)" {
		t.Fatalf("second copy: %d %s", two.StatusCode(), two.Body)
	}
	paused := must(c.UpdateHelperWithResponse(ctx, wsID(h.ID), api.UpdateHelperJSONRequestBody{Name: new("Briefing Laden"),
		Status: new(api.UpdateHelperJSONBodyStatus("paused"))}))
	if paused.StatusCode() != 200 || paused.JSON200.Name != "Briefing Laden" || paused.JSON200.Status != "paused" || paused.JSON200.NextRunAt != nil {
		t.Fatalf("pause: %d %s", paused.StatusCode(), paused.Body)
	}
	if bad := must(c.UpdateHelperWithResponse(ctx, wsID(h.ID), api.UpdateHelperJSONRequestBody{Name: new("  ")})); bad.StatusCode() != 400 {
		t.Fatalf("blank name: %d", bad.StatusCode())
	}
	benC := e.client(t, "ben")
	if r := must(benC.DeleteHelperWithResponse(ctx, wsID(h.ID))); r.StatusCode() != 404 {
		t.Fatalf("ben deletes: %d", r.StatusCode())
	}
	if r := must(c.DeleteHelperWithResponse(ctx, wsID(h.ID))); r.StatusCode() != 204 {
		t.Fatalf("delete: %d", r.StatusCode())
	}
	if r := must(c.GetHelperWithResponse(ctx, wsID(h.ID))); r.StatusCode() != 404 {
		t.Fatalf("deleted helper: %d", r.StatusCode())
	}
	if _, err := st.GetRun(ctx, r.ID); err == nil {
		t.Fatal("runs of a deleted helper remain")
	}

	if r := must(benC.StartRunWithResponse(ctx, wsID(h.ID), api.StartRunJSONRequestBody{})); r.StatusCode() != 404 {
		t.Fatalf("ben runs: %d", r.StatusCode())
	}
	if r := must(benC.ListTemplatesWithResponse(ctx, wsID(ws.ID))); r.StatusCode() != 404 {
		t.Fatalf("ben shelf: %d", r.StatusCode())
	}
}

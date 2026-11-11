package shelf_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/shelf"
	"github.com/STGime/laamo/backend/internal/store"
)

var cat = catalog.Default()

func tpl(t *testing.T, slug string) catalog.Template {
	t.Helper()
	tp, ok := shelf.Find(slug)
	if !ok {
		t.Fatalf("no template %s", slug)
	}
	return tp
}

func TestPreview(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "de", Profile: map[string]any{"business": "Bäckerei"}, Needs: []string{"plan_events", "stay_up_to_date"}}
	v := shelf.Preview(cat, tpl(t, "morning-briefing"), ws)
	if v.Name != "Morgen-Briefing" || !v.Brain || !slices.Contains(v.Places, "slack") {
		t.Fatalf("view %+v", v)
	}
	// Known facts are filled in; unknown ones stay as pills and are asked.
	if !strings.Contains(v.Bricks[1].Sentence, "Bäckerei {my city} Neuigkeiten") || v.Bricks[1].Group != "look" {
		t.Fatalf("sentence %q", v.Bricks[1].Sentence)
	}
	keys := []string{}
	for _, q := range v.Questions {
		keys = append(keys, q.Key)
	}
	if !slices.Equal(keys, []string{"city", "chat"}) || v.Questions[0].Text != "Wo bist du zu Hause?" || !v.Questions[0].Profile {
		t.Fatalf("questions %+v", v.Questions)
	}

	// Needs rank the shelf: Weekend Scout (plan_events, picked first) leads.
	views := []shelf.View{v, shelf.Preview(cat, tpl(t, "weekend-scout"), ws)}
	shelf.Sort(views)
	if views[0].Template.Slug != "weekend-scout" || views[0].Match <= views[1].Match {
		t.Fatalf("order %s %s", views[0].Template.Slug, views[1].Template.Slug)
	}
}

func TestInstallFeedOnly(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "de", Profile: map[string]any{"business": "Bäckerei"}}
	stack, profile, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, map[string]string{"city": "Leipzig", "chat": shelf.FeedOnly}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if profile["city"] != "Leipzig" || len(profile) != 1 {
		t.Fatalf("profile %v", profile)
	}
	if slices.ContainsFunc(stack.Bricks, func(b catalog.StackBrick) bool { return b.Type == "do.chat" }) {
		t.Fatal("Post to chat should be left out")
	}
	if q, _ := stack.Bricks[1].Config["query"].(string); q != "{my business} {my city} Neuigkeiten" {
		t.Fatalf("query %q", q)
	}
}

func TestInstallPlace(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "en", Profile: map[string]any{"business": "bakery", "city": "Leipzig"}}
	places := map[string]store.Place{"p1": {ID: "p1", WorkspaceID: "w1", Type: "slack"}, "p2": {ID: "p2", WorkspaceID: "other"}}
	lookup := func(id string) (store.Place, error) {
		if p, ok := places[id]; ok {
			return p, nil
		}
		return store.Place{}, store.ErrNotFound
	}
	stack, profile, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, map[string]string{"chat": "p1"}, lookup)
	if err != nil || len(profile) != 0 {
		t.Fatalf("%v %v", err, profile)
	}
	if stack.Bricks[4].Type != "do.chat" || stack.Bricks[4].Config["place"] != "p1" {
		t.Fatalf("chat brick %+v", stack.Bricks[4])
	}
	for _, answers := range []map[string]string{{"chat": "p2"}, {"chat": "nope"}, {}} {
		if _, _, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, answers, lookup); !errors.Is(err, shelf.ErrAnswer) {
			t.Errorf("%v: %v", answers, err)
		}
	}
	// A missing About me fact is an error, not an empty search.
	ws.Profile = map[string]any{}
	if _, _, err := shelf.Install(cat, tpl(t, "weekend-scout"), ws, map[string]string{}, lookup); !errors.Is(err, shelf.ErrAnswer) {
		t.Fatalf("missing city: %v", err)
	}
}

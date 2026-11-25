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
	stack, profile, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, map[string]string{"city": "Leipzig", "chat": shelf.FeedOnly}, nil, nil)
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
	stack, profile, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, map[string]string{"chat": "p1"}, nil, lookup)
	if err != nil || len(profile) != 0 {
		t.Fatalf("%v %v", err, profile)
	}
	if stack.Bricks[4].Type != "do.chat" || stack.Bricks[4].Config["place"] != "p1" {
		t.Fatalf("chat brick %+v", stack.Bricks[4])
	}
	for _, answers := range []map[string]string{{"chat": "p2"}, {"chat": "nope"}, {}} {
		if _, _, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws, answers, nil, lookup); !errors.Is(err, shelf.ErrAnswer) {
			t.Errorf("%v: %v", answers, err)
		}
	}
	// A missing About me fact is an error, not an empty search.
	ws.Profile = map[string]any{}
	if _, _, err := shelf.Install(cat, tpl(t, "weekend-scout"), ws, map[string]string{}, nil, lookup); !errors.Is(err, shelf.ErrAnswer) {
		t.Fatalf("missing city: %v", err)
	}
}

// "Just for this helper": the fact stays with the helper, the workspace
// profile isn't touched, and the bricks read it.
func TestInstallJustThisHelper(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "en", Profile: map[string]any{"business": "bakery"}}
	stack, profile, err := shelf.Install(cat, tpl(t, "morning-briefing"), ws,
		map[string]string{"city": "Hamburg", "chat": shelf.FeedOnly}, []string{"city"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile) != 0 || stack.Profile["city"] != "Hamburg" {
		t.Fatalf("profile %v, stack profile %v", profile, stack.Profile)
	}
	if got := shelf.Describe(cat, stack, ws)[1].Sentence; !strings.Contains(got, "bakery Hamburg") {
		t.Fatalf("sentence %q", got)
	}
}

// About me questions carry the fact's hint and example (e.g. "the more you
// say about your offer, the better").
func TestQuestionHint(t *testing.T) {
	cat := catalog.Default()
	tpl, ok := shelf.Find("customer-finder")
	if !ok {
		t.Fatal("no customer-finder")
	}
	v := shelf.Preview(cat, tpl, store.Workspace{Locale: "de", Profile: map[string]any{}})
	for _, q := range v.Questions {
		if q.Key == "offer" {
			if !strings.Contains(q.Help, "Je genauer") || q.Placeholder == "" {
				t.Fatalf("offer question %+v", q)
			}
			return
		}
	}
	t.Fatal("no offer question")
}

// An answer can go into a longer setting: {value} is the answer, and one
// answer can fill several settings.
func TestInstallFillText(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "de", Profile: map[string]any{"business": "Malerbetrieb"}}
	stack, _, err := shelf.Install(cat, tpl(t, "tender-finder"), ws, map[string]string{"region": "Sachsen", "chat": shelf.FeedOnly}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q, _ := stack.Bricks[1].Config["query"].(string); q != "öffentliche Ausschreibung {my business} Sachsen" {
		t.Fatalf("query %q", q)
	}
	if c, _ := stack.Bricks[2].Config["criteria"].(string); !strings.Contains(c, "in Sachsen sind") {
		t.Fatalf("criteria %q", c)
	}
}

// Every template installs with plain answers.
func TestInstallAll(t *testing.T) {
	templates, err := catalog.Templates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range templates {
		answers := map[string]string{}
		for _, q := range tp.SetupQuestions {
			switch {
			case q.Profile != "":
				answers[q.Profile] = "Bäckerei"
			case q.Type == "place":
				answers[q.Key] = shelf.FeedOnly
			case q.Type == "channels":
				answers[q.Key] = "11, 12"
			case q.Key == "pages":
				answers[q.Key] = "https://shop.example/a https://shop.example/b"
			default:
				answers[q.Key] = "Leipzig"
			}
		}
		for _, lang := range []string{"en", "de"} {
			if _, _, err := shelf.Install(cat, tp, store.Workspace{ID: "w1", Locale: lang}, answers, nil, nil); err != nil {
				t.Errorf("%s (%s): %v", tp.Slug, lang, err)
			}
		}
	}
}

// Social Poster: the accounts picked become the brick's channels, the topic
// goes into what to write, and anything but account ids is refused.
func TestInstallSocialPoster(t *testing.T) {
	ws := store.Workspace{ID: "w1", Locale: "en", Profile: map[string]any{"business": "Bakery Krause"}}
	answers := map[string]string{"topic": "our bread of the week", "accounts": "11, 12"}
	stack, _, err := shelf.Install(cat, tpl(t, "social-poster"), ws, answers, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	last := stack.Bricks[len(stack.Bricks)-1]
	if ids, _ := last.Config["channels"].([]any); last.Type != "do.social" || len(ids) != 2 || ids[0] != "11" || ids[1] != "12" {
		t.Fatalf("channels %+v", last.Config)
	}
	if what, _ := stack.Bricks[1].Config["what"].(string); !strings.Contains(what, "about: our bread of the week.") {
		t.Fatalf("what %q", what)
	}
	// It asks before it posts.
	if stack.Bricks[len(stack.Bricks)-2].Type != "ask.check" {
		t.Fatalf("no check before the post: %+v", stack.Bricks)
	}
	for _, bad := range []string{"", "abc", "11;drop", "11,x"} {
		answers["accounts"] = bad
		if _, _, err := shelf.Install(cat, tpl(t, "social-poster"), ws, answers, nil, nil); err == nil {
			t.Errorf("accounts %q was taken", bad)
		}
	}
}

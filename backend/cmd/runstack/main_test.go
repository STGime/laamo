package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Morning Briefing runs end to end offline (fixture search and AI).
func TestMorningBriefingOffline(t *testing.T) {
	var out strings.Builder
	err := run(context.Background(), &out, "../../../packages/catalog/templates/morning-briefing.json",
		options{profile: "business=bakery,city=Leipzig", lang: "en", tz: "Europe/Berlin"})
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"✓ b5", "DONE", "Feed: Morning Briefing · Done", "printed, not sent", "Flour prices"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

func TestInvalidStackAndSets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.json")
	_ = os.WriteFile(path, []byte(`{"schema":1,"bricks":[{"id":"b1","type":"when.tap","config":{"question":"Which city?"}},
		{"id":"b2","type":"look.search","config":{"query":"bakeries in {answer}"}}]}`), 0o600)

	var out strings.Builder
	if err := run(context.Background(), &out, path, options{lang: "en", tz: "UTC", answer: "Dresden", sets: []string{"b2.count=5"}}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "(0 → 5 cards") {
		t.Fatalf("-set count=5 not applied:\n%s", out.String())
	}

	out.Reset()
	_ = os.WriteFile(path, []byte(`{"schema":1,"bricks":[{"id":"b1","type":"look.search","config":{}}]}`), 0o600)
	if err := run(context.Background(), &out, path, options{lang: "en", tz: "UTC"}); err == nil || !strings.Contains(out.String(), "start_first") {
		t.Fatalf("invalid stack: %v\n%s", err, out.String())
	}
}

// A German workspace gets the German template text.
func TestMorningBriefingGerman(t *testing.T) {
	var out strings.Builder
	err := run(context.Background(), &out, "../../../packages/catalog/templates/morning-briefing.json",
		options{profile: "business=Bäckerei,city=Leipzig", lang: "de", tz: "Europe/Berlin"})
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"Neuigkeiten", "am wichtigsten sind", "DONE"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "matter most") {
		t.Errorf("English text in a German run:\n%s", got)
	}
}

// A typo like "-lang de," is refused before anything reaches Brave.
func TestBadLanguage(t *testing.T) {
	var out strings.Builder
	err := run(context.Background(), &out, "../../../packages/catalog/templates/morning-briefing.json", options{lang: "de,", tz: "Europe/Berlin"})
	if err == nil || !strings.Contains(err.Error(), "use en or de") {
		t.Fatalf("err %v", err)
	}
}

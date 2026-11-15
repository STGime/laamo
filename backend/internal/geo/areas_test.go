package geo

import (
	"strings"
	"testing"
)

func TestAreas(t *testing.T) {
	for where, want := range map[string]string{
		"Leipzig":                          "Leipzig",
		"Berlin, Germany":                  "Berlin, Germany",
		"Sachsen":                          "Sachsen",
		"Germany, Austria":                 "Deutschland|Österreich",
		"DACH":                             "Deutschland|Österreich|Schweiz",
		"Leipzig und Dresden":              "Leipzig|Dresden",
		"Leipzig; Halle (Saale)":           "Leipzig|Halle (Saale)",
		"Benelux + Deutschland":            "Belgien|Niederlande|Luxemburg|Deutschland",
		"Deutschland, Österreich, Germany": "Deutschland|Österreich",
		"":                                 "",
	} {
		if got := strings.Join(Areas(where), "|"); got != want {
			t.Errorf("%q: %q, want %q", where, got, want)
		}
	}
	if n := len(Areas("EU")); n != 27 {
		t.Fatalf("EU: %d countries", n)
	}
	if n := len(Areas("EU, Schweiz, Norwegen, UK")); n != MaxAreas || Areas("EU, Schweiz, Norwegen, UK")[27] != "Schweiz" {
		t.Fatalf("EU + 3: %d", n)
	}
}

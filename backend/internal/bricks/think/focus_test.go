package think

import (
	"fmt"
	"strings"
	"testing"
)

func menu() string {
	var b strings.Builder
	b.WriteString("Willkommen bei Da Mario\nÖffnungszeiten: Mo–So 11–22 Uhr\n\nPIZZA\n")
	for i := range 60 {
		fmt.Fprintf(&b, "%d. Pizza Sorte %d\nmit Tomaten, Käse und Zutat %d\n%d,50 €\n", i, i, i, 9+i%5)
	}
	b.WriteString("31a. Pizza Margherita\nmit Tomaten und Mozzarella\n7,50 €\n32a. Pizza Salami\nmit Tomaten, Käse und Salami\n8,90 €\nGetränke\nCola 0,33 l\n2,50 €\n")
	return b.String()
}

// A long menu is cut down to the lines about the pizza asked for: its
// price reaches the model although it stands far down the page.
func TestFocus(t *testing.T) {
	m := menu()
	if n := len([]rune(m)); n < 3000 {
		t.Fatalf("the menu is only %d characters", n)
	}
	got := focus(m, "Pizza Salami", 1500)
	if len([]rune(got)) > 1500 || !strings.Contains(got, "32a. Pizza Salami\nmit Tomaten, Käse und Salami\n8,90 €") {
		t.Fatalf("salami:\n%s", got)
	}
	// Spelled the way people type it.
	if got := focus(m, "pizza margarita", 1500); !strings.Contains(got, "Pizza Margherita\nmit Tomaten und Mozzarella\n7,50 €") {
		t.Fatalf("margarita:\n%s", got)
	}
	// Nothing about it, or short anyway: left alone.
	if got := focus(m, "Sushi", 1500); got != m {
		t.Fatal("a text without a match was cut")
	}
	if got := focus("Pizza Salami 8,90 €\nCola 2,50 €", "Salami", 1500); got != "Pizza Salami 8,90 €\nCola 2,50 €" {
		t.Fatalf("a short text was cut: %q", got)
	}
}

func TestAmount(t *testing.T) {
	for in, want := range map[string]float64{"8,50 €": 8.5, "EUR 12.90": 12.9, "ab 7 €": 7, "9.00€ (klein)": 9} {
		if got, ok := amount(in); !ok || got != want {
			t.Errorf("%q: %v %v", in, got, ok)
		}
	}
	if _, ok := amount("auf Anfrage"); ok {
		t.Error("a price without a number")
	}
}

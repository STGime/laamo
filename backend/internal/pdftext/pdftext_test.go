package pdftext_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/STGime/laamo/backend/internal/pdftext"
	"github.com/STGime/laamo/backend/internal/pdftext/pdftexttest"
)

// A menu comes out line by line: the dish and its price together.
func TestText(t *testing.T) {
	data := pdftexttest.Make("Speisekarte", "Pizza Margherita\t7,50 EUR", "Pizza Salami\t8,90 EUR", "Cola 0,33 l\t2,50 EUR")
	if !pdftext.Is(data) {
		t.Fatal("not taken for a PDF")
	}
	got, err := pdftext.Text(data, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Speisekarte\nPizza Margherita 7,50 EUR\nPizza Salami 8,90 EUR\nCola 0,33 l 2,50 EUR" {
		t.Fatalf("text %q", got)
	}
	if short, _ := pdftext.Text(data, 20); len([]rune(short)) > 30 {
		t.Fatalf("not cut: %q", short)
	}
}

// A file that isn't one, a broken one and one without text give an error.
func TestTextRefuses(t *testing.T) {
	if pdftext.Is([]byte("<html>")) {
		t.Fatal("html taken for a PDF")
	}
	for name, data := range map[string][]byte{
		"not a pdf": []byte("hello"),
		"cut off":   pdftexttest.Make("Pizza Salami\t8,90 EUR")[:200],
		"garbage":   append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0xff, 0x00, 0x13}, 400)...),
	} {
		if text, err := pdftext.Text(data, 5000); err == nil {
			t.Errorf("%s: no error, text %q", name, text)
		}
	}
	if _, err := pdftext.Text(pdftexttest.Make(), 5000); !errors.Is(err, pdftext.ErrNoText) {
		t.Errorf("an empty page: %v", err)
	}
}

package api

import (
	"strings"
	"testing"
)

func TestIgnoredText(t *testing.T) {
	for _, c := range []struct {
		de, listed, signed bool
		want               string
	}{
		{false, true, false, "didn't sign it"},
		{false, false, true, "Allow it if you know the sender"},
		{false, false, false, "can't check that the email really came from there"},
		{true, false, true, "Erlaube sie"},
	} {
		title, body := ignoredText(c.de, "x@example.com", c.listed, c.signed)
		if title == "" || !strings.Contains(body, "x@example.com") || !strings.Contains(body, c.want) {
			t.Errorf("%+v: %q %q", c, title, body)
		}
	}
	if _, body := ignoredText(false, strings.Repeat("a", 500)+"@x.de", false, false); len([]rune(body)) > 300 {
		t.Errorf("long sender not cut: %d", len(body))
	}
}

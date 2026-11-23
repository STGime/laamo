package think

import (
	"testing"

	"github.com/STGime/laamo/backend/internal/engine"
)

func TestCheckAmount(t *testing.T) {
	c := engine.Card{}
	c.Set("photo_text", "CAFE SONNE\nNetto 7% 10,75\nMwSt 7% 0,75\nSUMME EUR 11,50\nTaxi 12,-\nGesamt 1.234,56", engine.TrustUntrusted)
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"11.50", "11.50", true},
		{"11,50 €", "11.50", true},
		{"EUR 11.5", "11.50", true},
		{"0.75", "0.75", true},
		{"12.00", "12.00", true}, // printed as "12,-"
		{"12", "12", true},
		{"1,234.56", "1234.56", true},
		{"1.234,56", "1234.56", true},
		{"19.99", "", false}, // not on the receipt: the AI made it up
		{"11.57", "", false},
		{"about ten", "", false},
		{"0", "", false},
	} {
		got, ok := checkDetail("total", tc.in, c)
		if got != tc.want || ok != tc.ok {
			t.Errorf("checkDetail(total, %q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	// Other receipt details are free text.
	if v, ok := checkDetail("vendor", "Café Sonne", c); !ok || v != "Café Sonne" {
		t.Errorf("vendor = %q, %v", v, ok)
	}
}

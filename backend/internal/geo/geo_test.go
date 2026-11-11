package geo_test

import (
	"testing"

	"github.com/STGime/laamo/backend/internal/geo"
)

func TestCountry(t *testing.T) {
	for _, c := range []struct{ city, tz, want string }{
		{"Berlin, Germany", "Europe/Berlin", "DE"},
		{"Berlin", "America/New_York", "DE"}, // the city wins over the laptop's clock
		{"Wien", "Europe/Berlin", "AT"},
		{"Graz (Österreich)", "", "AT"},
		{"Zürich", "Europe/Berlin", "CH"},
		{"Leipzig, DE", "", "DE"},
		{"Kleinkleckersdorf", "Europe/Vienna", "AT"}, // unknown town: the timezone
		{"", "Europe/Paris", "FR"},
		{"Dublin, Ireland", "Europe/Dublin", ""}, // Brave has no Ireland
		{"", "Asia/Kolkata", ""},
		{"St. Gallen", "", "CH"},
	} {
		if got := geo.Country(c.city, c.tz); got != c.want {
			t.Errorf("Country(%q, %q) = %q, want %q", c.city, c.tz, got, c.want)
		}
	}
	for code := range map[string]string{"DE": "", "AT": "", "CH": ""} {
		if !geo.Supported[code] {
			t.Errorf("%s must be supported", code)
		}
	}
}

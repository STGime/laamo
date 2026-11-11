// Package geo guesses a workspace's country for local search results, from
// what it already knows: the About me city and the browser's timezone.
package geo

import (
	"strings"
	"unicode"
)

// Supported are the country codes Brave Search accepts; it refuses others.
var Supported = map[string]bool{
	"AR": true, "AU": true, "AT": true, "BE": true, "BR": true, "CA": true, "CL": true, "DK": true, "FI": true,
	"FR": true, "DE": true, "HK": true, "IN": true, "ID": true, "IT": true, "JP": true, "KR": true, "MY": true,
	"MX": true, "NL": true, "NZ": true, "NO": true, "CN": true, "PL": true, "PT": true, "PH": true, "RU": true,
	"SA": true, "ZA": true, "ES": true, "SE": true, "CH": true, "TW": true, "TR": true, "GB": true, "US": true,
}

// Country names as people type them after a city ("Berlin, Germany").
var countryNames = map[string]string{
	"germany": "DE", "deutschland": "DE", "austria": "AT", "österreich": "AT", "oesterreich": "AT",
	"switzerland": "CH", "schweiz": "CH", "suisse": "CH", "svizzera": "CH", "france": "FR", "frankreich": "FR",
	"netherlands": "NL", "niederlande": "NL", "nederland": "NL", "belgium": "BE", "belgien": "BE", "belgique": "BE",
	"italy": "IT", "italien": "IT", "italia": "IT", "spain": "ES", "spanien": "ES", "españa": "ES",
	"portugal": "PT", "poland": "PL", "polen": "PL", "polska": "PL", "denmark": "DK", "dänemark": "DK",
	"danmark": "DK", "sweden": "SE", "schweden": "SE", "sverige": "SE", "norway": "NO", "norwegen": "NO",
	"norge": "NO", "finland": "FI", "finnland": "FI", "suomi": "FI", "uk": "GB", "united kingdom": "GB",
	"england": "GB", "scotland": "GB", "wales": "GB", "großbritannien": "GB", "usa": "US", "united states": "US",
	"canada": "CA", "kanada": "CA", "turkey": "TR", "türkei": "TR", "türkiye": "TR",
}

// Cities whose timezone may say otherwise (someone in Vienna with a laptop
// set to Berlin time).
var cities = map[string]string{
	"wien": "AT", "vienna": "AT", "graz": "AT", "linz": "AT", "salzburg": "AT", "innsbruck": "AT", "klagenfurt": "AT",
	"zürich": "CH", "zurich": "CH", "zuerich": "CH", "basel": "CH", "bern": "CH", "genf": "CH", "geneva": "CH",
	"genève": "CH", "lausanne": "CH", "luzern": "CH", "lucerne": "CH", "st. gallen": "CH", "winterthur": "CH",
	"berlin": "DE", "hamburg": "DE", "münchen": "DE", "munich": "DE", "köln": "DE", "cologne": "DE",
	"frankfurt": "DE", "leipzig": "DE", "dresden": "DE", "stuttgart": "DE", "düsseldorf": "DE",
	"amsterdam": "NL", "rotterdam": "NL", "brussels": "BE", "brüssel": "BE", "bruxelles": "BE",
	"paris": "FR", "lyon": "FR", "copenhagen": "DK", "kopenhagen": "DK", "stockholm": "SE", "oslo": "NO",
	"helsinki": "FI", "warsaw": "PL", "warschau": "PL", "lisbon": "PT", "lissabon": "PT", "madrid": "ES",
	"barcelona": "ES", "rome": "IT", "rom": "IT", "milan": "IT", "mailand": "IT", "london": "GB",
}

// Timezones of the countries search supports (Europe first: that's who
// Laamo is for).
var zones = map[string]string{
	"Europe/Berlin": "DE", "Europe/Busingen": "DE", "Europe/Vienna": "AT", "Europe/Zurich": "CH",
	"Europe/Paris": "FR", "Europe/Amsterdam": "NL", "Europe/Brussels": "BE", "Europe/Rome": "IT",
	"Europe/Madrid": "ES", "Europe/Lisbon": "PT", "Europe/Warsaw": "PL", "Europe/Copenhagen": "DK",
	"Europe/Stockholm": "SE", "Europe/Oslo": "NO", "Europe/Helsinki": "FI", "Europe/London": "GB",
	"Europe/Istanbul": "TR", "America/New_York": "US", "America/Chicago": "US", "America/Denver": "US",
	"America/Los_Angeles": "US", "America/Toronto": "CA", "America/Vancouver": "CA", "Australia/Sydney": "AU",
}

// Country returns a supported country code for local results, or "" when
// it can't tell.
func Country(city, timezone string) string {
	c := strings.ToLower(strings.TrimSpace(city))
	if c != "" {
		// "Berlin, Germany", "Graz (Österreich)": a country named last wins.
		parts := strings.FieldsFunc(c, func(r rune) bool { return r == ',' || r == '(' || r == ')' || r == '/' })
		for i := len(parts) - 1; i >= 0; i-- {
			p := strings.TrimSpace(parts[i])
			if code, ok := countryNames[p]; ok {
				return code
			}
			if len(p) == 2 && isLetters(p) && Supported[strings.ToUpper(p)] && i > 0 {
				return strings.ToUpper(p) // "Leipzig, DE"
			}
		}
		if code, ok := cities[strings.TrimSpace(parts[0])]; ok {
			return code
		}
	}
	return zones[timezone]
}

func isLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

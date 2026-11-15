package geo

import (
	"strings"
)

// Countries Find businesses can search across, by the names people type
// (English and German) → the name to look up on the map.
var europe = map[string][]string{
	"Deutschland": {"germany", "deutschland"}, "Österreich": {"austria", "österreich", "oesterreich"},
	"Schweiz": {"switzerland", "schweiz"}, "Frankreich": {"france", "frankreich"}, "Italien": {"italy", "italien"},
	"Spanien": {"spain", "spanien"}, "Portugal": {"portugal"}, "Niederlande": {"netherlands", "niederlande", "holland"},
	"Belgien": {"belgium", "belgien"}, "Luxemburg": {"luxembourg", "luxemburg"}, "Dänemark": {"denmark", "dänemark"},
	"Schweden": {"sweden", "schweden"}, "Finnland": {"finland", "finnland"}, "Norwegen": {"norway", "norwegen"},
	"Island": {"iceland", "island"}, "Irland": {"ireland", "irland"}, "Polen": {"poland", "polen"},
	"Tschechien": {"czechia", "czech republic", "tschechien"}, "Slowakei": {"slovakia", "slowakei"},
	"Ungarn": {"hungary", "ungarn"}, "Slowenien": {"slovenia", "slowenien"}, "Kroatien": {"croatia", "kroatien"},
	"Rumänien": {"romania", "rumänien"}, "Bulgarien": {"bulgaria", "bulgarien"}, "Griechenland": {"greece", "griechenland"},
	"Zypern": {"cyprus", "zypern"}, "Malta": {"malta"}, "Estland": {"estonia", "estland"}, "Lettland": {"latvia", "lettland"},
	"Litauen": {"lithuania", "litauen"}, "Liechtenstein": {"liechtenstein"},
	"Vereinigtes Königreich": {"united kingdom", "uk", "großbritannien", "vereinigtes königreich", "great britain"},
}

// Groups of countries people name as one.
var groups = map[string][]string{
	"eu": {"Deutschland", "Österreich", "Frankreich", "Italien", "Spanien", "Portugal", "Niederlande", "Belgien",
		"Luxemburg", "Dänemark", "Schweden", "Finnland", "Irland", "Polen", "Tschechien", "Slowakei", "Ungarn",
		"Slowenien", "Kroatien", "Rumänien", "Bulgarien", "Griechenland", "Zypern", "Malta", "Estland", "Lettland", "Litauen"},
	"dach":     {"Deutschland", "Österreich", "Schweiz"},
	"benelux":  {"Belgien", "Niederlande", "Luxemburg"},
	"nordics":  {"Dänemark", "Schweden", "Norwegen", "Finnland", "Island"},
	"baltics":  {"Estland", "Lettland", "Litauen"},
	"baltikum": {"Estland", "Lettland", "Litauen"},
}

var groupNames = map[string]string{
	"europäische union": "eu", "european union": "eu", "eu": "eu", "die eu": "eu", "the eu": "eu",
	"dach": "dach", "d-a-ch": "dach", "benelux": "benelux", "nordics": "nordics", "skandinavien": "nordics",
	"scandinavia": "nordics", "nordeuropa": "nordics", "baltics": "baltics", "baltikum": "baltikum",
}

// MaxAreas: a search looks in at most this many places.
const MaxAreas = 30

// Areas splits where someone wants to look into places to look up:
// "EU" → its 27 countries; "Deutschland, Österreich" → two countries;
// "Leipzig; Dresden" or "Leipzig und Dresden" → two cities. A comma only
// separates places when every part is a country or group, so
// "Berlin, Germany" stays one place.
func Areas(where string) []string {
	where = strings.TrimSpace(where)
	if where == "" {
		return nil
	}
	var parts []string
	for _, p := range strings.FieldsFunc(where, func(r rune) bool { return r == ';' || r == '\n' || r == '+' }) {
		parts = append(parts, splitWords(p, " und ", " and ", " & ")...)
	}
	var out []string
	for _, p := range parts {
		commas := strings.Split(p, ",")
		if len(commas) > 1 && allKnown(commas) {
			for _, c := range commas {
				out = append(out, expand(c)...)
			}
			continue
		}
		out = append(out, expand(p)...)
	}
	// No twice, at most MaxAreas.
	seen := map[string]bool{}
	var uniq []string
	for _, a := range out {
		if a = strings.TrimSpace(a); a != "" && !seen[strings.ToLower(a)] && len(uniq) < MaxAreas {
			seen[strings.ToLower(a)] = true
			uniq = append(uniq, a)
		}
	}
	return uniq
}

func splitWords(s string, seps ...string) []string {
	out := []string{s}
	for _, sep := range seps {
		var next []string
		for _, p := range out {
			next = append(next, strings.Split(p, sep)...)
		}
		out = next
	}
	return out
}

func known(s string) bool {
	k := strings.ToLower(strings.TrimSpace(s))
	if _, ok := groupNames[k]; ok {
		return true
	}
	return country(k) != ""
}

func allKnown(parts []string) bool {
	for _, p := range parts {
		if !known(p) {
			return false
		}
	}
	return true
}

func country(lower string) string {
	for name, words := range europe {
		for _, w := range words {
			if w == lower {
				return name
			}
		}
	}
	return ""
}

// expand turns a group into its countries and a country name into the
// name the map knows; anything else stays as typed.
func expand(s string) []string {
	k := strings.ToLower(strings.TrimSpace(s))
	if g, ok := groupNames[k]; ok {
		return groups[g]
	}
	if c := country(k); c != "" {
		return []string{c}
	}
	return []string{strings.TrimSpace(s)}
}

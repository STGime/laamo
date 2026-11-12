package osm

import (
	"regexp"
	"slices"
	"strings"
)

// Tag is one OpenStreetMap tag, e.g. shop=bakery.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Keys a business can be tagged with. Tags from anywhere else (the AI) must
// use one of these, so a query can't ask for something odd.
var Keys = map[string]bool{
	"shop": true, "amenity": true, "craft": true, "office": true, "tourism": true,
	"leisure": true, "healthcare": true, "club": true,
}

var valueRe = regexp.MustCompile(`^[a-z0-9_]{2,40}$`)

// ValidTag reports whether a tag is safe to put in a query.
func ValidTag(t Tag) bool {
	return Keys[t.Key] && valueRe.MatchString(t.Value)
}

func tags(kv ...string) []Tag {
	out := make([]Tag, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, Tag{kv[i], kv[i+1]})
	}
	return out
}

// kinds: what people type (English and German, any form) → tags. The words
// are matched after normalising (lower case, ä→ae, plural endings off).
var kinds = []struct {
	words []string
	tags  []Tag
}{
	{[]string{"bakery", "bakeries", "baeckerei", "baecker", "backstube"}, tags("shop", "bakery")},
	{[]string{"pastry", "konditorei", "patisserie", "cake shop"}, tags("shop", "pastry", "shop", "confectionery")},
	{[]string{"butcher", "metzger", "metzgerei", "fleischerei", "schlachterei"}, tags("shop", "butcher")},
	{[]string{"cafe", "café", "coffee shop", "kaffee", "kaffeehaus"}, tags("amenity", "cafe")},
	{[]string{"restaurant", "gaststaette", "gasthaus", "lokal"}, tags("amenity", "restaurant")},
	{[]string{"bar", "kneipe", "pub", "bars"}, tags("amenity", "bar", "amenity", "pub")},
	{[]string{"fast food", "imbiss", "snack bar", "doener", "kebab", "pizzeria"}, tags("amenity", "fast_food")},
	{[]string{"ice cream", "eisdiele", "eiscafe", "gelato"}, tags("amenity", "ice_cream", "shop", "ice_cream")},
	{[]string{"hotel", "hotels"}, tags("tourism", "hotel")},
	{[]string{"guest house", "pension", "gaestehaus", "b&b", "bed and breakfast"}, tags("tourism", "guest_house")},
	{[]string{"hairdresser", "hair salon", "barber", "friseur", "frisoer", "friseursalon", "barbershop"}, tags("shop", "hairdresser")},
	{[]string{"beauty salon", "kosmetik", "kosmetikstudio", "nail salon", "nagelstudio", "beauty"}, tags("shop", "beauty")},
	{[]string{"florist", "flower shop", "blumenladen", "blumen", "floristik"}, tags("shop", "florist")},
	{[]string{"bookshop", "bookstore", "buchhandlung", "buchladen"}, tags("shop", "books")},
	{[]string{"clothes", "clothing store", "fashion", "boutique", "bekleidung", "modegeschaeft"}, tags("shop", "clothes")},
	{[]string{"shoe shop", "schuhgeschaeft", "schuhe"}, tags("shop", "shoes")},
	{[]string{"jeweller", "jeweler", "juwelier", "schmuck"}, tags("shop", "jewelry")},
	{[]string{"optician", "optiker"}, tags("shop", "optician")},
	{[]string{"supermarket", "supermarkt"}, //nolint:misspell // German
		tags("shop", "supermarket")},
	{[]string{"organic shop", "bioladen", "reformhaus"}, tags("shop", "health_food", "shop", "organic")},
	{[]string{"deli", "feinkost", "delicatessen"}, tags("shop", "deli")},
	{[]string{"wine shop", "weinhandlung", "weinladen", "wein"}, tags("shop", "wine")},
	{[]string{"greengrocer", "gemuesehaendler", "obst und gemuese"}, tags("shop", "greengrocer")},
	{[]string{"kiosk", "spaetkauf", "spaeti"}, tags("shop", "kiosk")},
	{[]string{"pharmacy", "apotheke", "chemist"}, tags("amenity", "pharmacy")},
	{[]string{"dentist", "zahnarzt", "zahnarztpraxis"}, tags("amenity", "dentist", "healthcare", "dentist")},
	{[]string{"doctor", "arzt", "arztpraxis", "physician", "hausarzt"}, tags("amenity", "doctors", "healthcare", "doctor")},
	{[]string{"physiotherapist", "physiotherapie", "physio", "krankengymnastik"}, tags("healthcare", "physiotherapist")},
	{[]string{"vet", "veterinary", "tierarzt", "tierarztpraxis"}, tags("amenity", "veterinary")},
	{[]string{"gym", "fitness", "fitnessstudio", "fitness studio"}, tags("leisure", "fitness_centre")},
	{[]string{"yoga studio", "yoga"}, tags("leisure", "fitness_centre")},
	{[]string{"kindergarten", "kita", "daycare", "kinderkrippe"}, tags("amenity", "kindergarten", "amenity", "childcare")},
	{[]string{"car repair", "garage", "autowerkstatt", "kfz werkstatt", "werkstatt", "mechanic"}, tags("shop", "car_repair")},
	{[]string{"car dealer", "autohaus", "autohaendler"}, tags("shop", "car")},
	{[]string{"bike shop", "bicycle shop", "fahrradladen", "fahrradgeschaeft", "fahrrad"}, tags("shop", "bicycle")},
	{[]string{"electrician", "elektriker", "elektroinstallateur"}, tags("craft", "electrician")},
	{[]string{"plumber", "klempner", "installateur", "sanitaer"}, tags("craft", "plumber")},
	{[]string{"carpenter", "tischler", "schreiner", "tischlerei", "schreinerei"}, tags("craft", "carpenter")},
	{[]string{"painter", "maler", "malerbetrieb"}, tags("craft", "painter")},
	{[]string{"roofer", "dachdecker"}, tags("craft", "roofer")},
	{[]string{"photographer", "fotograf", "fotostudio"}, tags("craft", "photographer", "shop", "photo")},
	{[]string{"tailor", "schneider", "schneiderei", "aenderungsschneiderei"}, tags("craft", "tailor", "shop", "tailor")},
	{[]string{"lawyer", "anwalt", "rechtsanwalt", "kanzlei", "law firm"}, tags("office", "lawyer")},
	{[]string{"accountant", "steuerberater", "tax advisor", "buchhaltung"}, tags("office", "tax_advisor", "office", "accountant")},
	{[]string{"architect", "architekt", "architekturbuero"}, tags("office", "architect")},
	{[]string{"estate agent", "real estate", "makler", "immobilienmakler", "immobilien"}, tags("office", "estate_agent")}, //nolint:misspell // German
	{[]string{"insurance", "versicherung", "versicherungsagentur"}, tags("office", "insurance")},
	{[]string{"it company", "software company", "it firma", "softwarefirma", "it dienstleister"}, tags("office", "it", "office", "company")},
	{[]string{"coworking", "co-working", "coworking space"}, tags("amenity", "coworking_space", "office", "coworking")},
	{[]string{"travel agency", "reisebuero"}, tags("shop", "travel_agency")},
	{[]string{"pet shop", "zoohandlung", "tierbedarf"}, tags("shop", "pet")},
	{[]string{"toy shop", "spielwaren", "spielzeugladen"}, tags("shop", "toys")},
	{[]string{"hardware store", "baumarkt", "eisenwaren"}, tags("shop", "hardware", "shop", "doityourself")},
	{[]string{"furniture store", "moebelhaus", "moebel"}, tags("shop", "furniture")},
	{[]string{"laundry", "waescherei", "reinigung", "dry cleaning"}, tags("shop", "laundry", "shop", "dry_cleaning")},
	{[]string{"tattoo", "tattoo studio", "taetowierer"}, tags("shop", "tattoo")},
	{[]string{"music school", "musikschule"}, tags("amenity", "music_school")},
	{[]string{"driving school", "fahrschule"}, tags("amenity", "driving_school")},
	{[]string{"language school", "sprachschule"}, tags("amenity", "language_school")},
}

// forms: what to compare a word by: lower case, ä→ae, and the word with
// one plural ending off, so "Bäckereien", "bakeries" and "Bakery" meet.
func forms(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "-", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return nil
	}
	out := []string{s}
	for _, suf := range []string{"eien", "ies", "en", "es", "s", "n", "e"} {
		if len(s) <= len(suf)+3 || !strings.HasSuffix(s, suf) {
			continue
		}
		switch suf {
		case "eien":
			out = append(out, s[:len(s)-4]+"ei")
		case "ies":
			out = append(out, s[:len(s)-3]+"y")
		default:
			out = append(out, s[:len(s)-len(suf)])
		}
	}
	return out
}

// Kinds maps what someone typed to tags, or nil when it's not a known kind.
func Kinds(what string) []Tag {
	typed := forms(what)
	if len(typed) == 0 {
		return nil
	}
	for _, k := range kinds {
		for _, word := range k.words {
			for _, f := range forms(word) {
				if slices.Contains(typed, f) {
					return k.tags
				}
			}
		}
	}
	return nil
}

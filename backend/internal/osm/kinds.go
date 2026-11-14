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

func cats(s string) []string { return strings.Split(s, ",") }

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
	geo   []string // Geoapify categories; nil: none fits, so it searches by name
}{
	{[]string{"bakery", "bakeries", "baeckerei", "baecker", "backstube"}, tags("shop", "bakery"), cats("commercial.food_and_drink.bakery")},
	{[]string{"pastry", "konditorei", "patisserie", "cake shop"}, tags("shop", "pastry", "shop", "confectionery"), cats("commercial.food_and_drink.confectionery,commercial.food_and_drink.bakery")},
	{[]string{"butcher", "metzger", "metzgerei", "fleischerei", "schlachterei"}, tags("shop", "butcher"), cats("commercial.food_and_drink.butcher")},
	{[]string{"cafe", "café", "coffee shop", "kaffee", "kaffeehaus"}, tags("amenity", "cafe"), cats("catering.cafe")},
	{[]string{"restaurant", "gaststaette", "gasthaus", "lokal"}, tags("amenity", "restaurant"), cats("catering.restaurant")},
	{[]string{"bar", "kneipe", "pub", "bars"}, tags("amenity", "bar", "amenity", "pub"), cats("catering.bar,catering.pub")},
	{[]string{"fast food", "imbiss", "snack bar", "doener", "kebab", "pizzeria"}, tags("amenity", "fast_food"), cats("catering.fast_food")},
	{[]string{"ice cream", "eisdiele", "eiscafe", "gelato"}, tags("amenity", "ice_cream", "shop", "ice_cream"), cats("catering.ice_cream,commercial.food_and_drink.ice_cream")},
	{[]string{"hotel", "hotels"}, tags("tourism", "hotel"), cats("accommodation.hotel")},
	{[]string{"guest house", "pension", "gaestehaus", "b&b", "bed and breakfast"}, tags("tourism", "guest_house"), cats("accommodation.guest_house")},
	{[]string{"hairdresser", "hair salon", "barber", "friseur", "frisoer", "friseursalon", "barbershop"}, tags("shop", "hairdresser"), cats("service.beauty.hairdresser")},
	{[]string{"beauty salon", "kosmetik", "kosmetikstudio", "nail salon", "nagelstudio", "beauty"}, tags("shop", "beauty"), cats("service.beauty")},
	{[]string{"florist", "flower shop", "blumenladen", "blumen", "floristik"}, tags("shop", "florist"), cats("commercial.florist")},
	{[]string{"bookshop", "bookstore", "buchhandlung", "buchladen"}, tags("shop", "books"), cats("commercial.books")},
	{[]string{"clothes", "clothing store", "fashion", "boutique", "bekleidung", "modegeschaeft"}, tags("shop", "clothes"), cats("commercial.clothing")},
	{[]string{"shoe shop", "schuhgeschaeft", "schuhe"}, tags("shop", "shoes"), cats("commercial.clothing.shoes")},
	{[]string{"jeweller", "jeweler", "juwelier", "schmuck"}, tags("shop", "jewelry"), cats("commercial.jewelry")},
	{[]string{"optician", "optiker"}, tags("shop", "optician"), cats("commercial.health_and_beauty.optician")},
	{[]string{"supermarket", "supermarkt"}, //nolint:misspell // German
		tags("shop", "supermarket"), cats("commercial.supermarket")},
	{[]string{"organic shop", "bioladen", "reformhaus"}, tags("shop", "health_food", "shop", "organic"), cats("commercial.food_and_drink.organic,commercial.food_and_drink.health_food")},
	{[]string{"deli", "feinkost", "delicatessen"}, tags("shop", "deli"), cats("commercial.food_and_drink.deli")},
	{[]string{"wine shop", "weinhandlung", "weinladen", "wein"}, tags("shop", "wine"), nil},
	{[]string{"greengrocer", "gemuesehaendler", "obst und gemuese"}, tags("shop", "greengrocer"), cats("commercial.food_and_drink.fruit_and_vegetable")},
	{[]string{"kiosk", "spaetkauf", "spaeti"}, tags("shop", "kiosk"), cats("commercial.kiosk")},
	{[]string{"pharmacy", "apotheke", "chemist"}, tags("amenity", "pharmacy"), cats("healthcare.pharmacy")},
	{[]string{"dentist", "zahnarzt", "zahnarztpraxis"}, tags("amenity", "dentist", "healthcare", "dentist"), cats("healthcare.dentist")},
	{[]string{"doctor", "arzt", "arztpraxis", "physician", "hausarzt"}, tags("amenity", "doctors", "healthcare", "doctor"), cats("healthcare.clinic_or_praxis")},
	{[]string{"physiotherapist", "physiotherapie", "physio", "krankengymnastik"}, tags("healthcare", "physiotherapist"), nil},
	{[]string{"vet", "veterinary", "tierarzt", "tierarztpraxis"}, tags("amenity", "veterinary"), cats("pet.veterinary")},
	{[]string{"gym", "fitness", "fitnessstudio", "fitness studio"}, tags("leisure", "fitness_centre"), nil},
	{[]string{"yoga studio", "yoga"}, tags("leisure", "fitness_centre"), nil},
	{[]string{"kindergarten", "kita", "daycare", "kinderkrippe"}, tags("amenity", "kindergarten", "amenity", "childcare"), cats("childcare.kindergarten")},
	{[]string{"car repair", "garage", "autowerkstatt", "kfz werkstatt", "werkstatt", "mechanic"}, tags("shop", "car_repair"), cats("service.vehicle.repair")},
	{[]string{"car dealer", "autohaus", "autohaendler"}, tags("shop", "car"), cats("commercial.vehicle")},
	{[]string{"bike shop", "bicycle shop", "fahrradladen", "fahrradgeschaeft", "fahrrad"}, tags("shop", "bicycle"), cats("commercial.outdoor_and_sport.bicycle")},
	{[]string{"electrician", "elektriker", "elektroinstallateur"}, tags("craft", "electrician"), cats("service.electrician")},
	{[]string{"plumber", "klempner", "installateur", "sanitaer"}, tags("craft", "plumber"), nil},
	{[]string{"carpenter", "tischler", "schreiner", "tischlerei", "schreinerei"}, tags("craft", "carpenter"), cats("service.carpenter")},
	{[]string{"painter", "maler", "malerbetrieb"}, tags("craft", "painter"), nil},
	{[]string{"roofer", "dachdecker"}, tags("craft", "roofer"), nil},
	{[]string{"photographer", "fotograf", "fotostudio"}, tags("craft", "photographer", "shop", "photo"), cats("service.photographer")},
	{[]string{"tailor", "schneider", "schneiderei", "aenderungsschneiderei"}, tags("craft", "tailor", "shop", "tailor"), cats("service.tailor")},
	{[]string{"lawyer", "anwalt", "rechtsanwalt", "kanzlei", "law firm"}, tags("office", "lawyer"), cats("office.lawyer")},
	{[]string{"accountant", "steuerberater", "tax advisor", "buchhaltung"}, tags("office", "tax_advisor", "office", "accountant"), cats("office.tax_advisor,office.accountant")},
	{[]string{"architect", "architekt", "architekturbuero"}, tags("office", "architect"), cats("office.architect")},
	{[]string{"estate agent", "real estate", "makler", "immobilienmakler", "immobilien"}, tags("office", "estate_agent"), cats("office.estate_agent,service.estate_agent")}, //nolint:misspell // German
	{[]string{"insurance", "versicherung", "versicherungsagentur"}, tags("office", "insurance"), cats("office.insurance")},
	{[]string{"it company", "software company", "it firma", "softwarefirma", "it dienstleister"}, tags("office", "it", "office", "company"), cats("office.it")},
	{[]string{"coworking", "co-working", "coworking space"}, tags("amenity", "coworking_space", "office", "coworking"), cats("office.coworking")},
	{[]string{"travel agency", "reisebuero"}, tags("shop", "travel_agency"), cats("service.travel_agency")},
	{[]string{"pet shop", "zoohandlung", "tierbedarf"}, tags("shop", "pet"), cats("pet.shop")},
	{[]string{"toy shop", "spielwaren", "spielzeugladen"}, tags("shop", "toys"), cats("commercial.toy_and_game")},
	{[]string{"hardware store", "baumarkt", "eisenwaren"}, tags("shop", "hardware", "shop", "doityourself"), cats("commercial.houseware_and_hardware")},
	{[]string{"furniture store", "moebelhaus", "moebel"}, tags("shop", "furniture"), cats("commercial.furniture_and_interior")},
	{[]string{"laundry", "waescherei", "reinigung", "dry cleaning"}, tags("shop", "laundry", "shop", "dry_cleaning"), cats("service.cleaning")},
	{[]string{"tattoo", "tattoo studio", "taetowierer"}, tags("shop", "tattoo"), cats("service.beauty.tattoo")},
	{[]string{"music school", "musikschule"}, tags("amenity", "music_school"), cats("education.music_school")},
	{[]string{"driving school", "fahrschule"}, tags("amenity", "driving_school"), cats("education.driving_school")},
	{[]string{"language school", "sprachschule"}, tags("amenity", "language_school"), cats("education.language_school")},
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

// Categories maps what someone typed to Geoapify categories, or nil.
func Categories(what string) []string {
	typed := forms(what)
	for _, k := range kinds {
		for _, word := range k.words {
			for _, f := range forms(word) {
				if slices.Contains(typed, f) {
					return k.geo
				}
			}
		}
	}
	return nil
}

// CategoriesFor maps OpenStreetMap tags (from the AI) to Geoapify
// categories, where a known kind has them.
func CategoriesFor(tags []Tag) []string {
	var out []string
	for _, t := range tags {
		for _, k := range kinds {
			if slices.Contains(k.tags, t) {
				for _, c := range k.geo {
					if !slices.Contains(out, c) {
						out = append(out, c)
					}
				}
			}
		}
	}
	return out
}

// Package osm finds businesses via Nominatim and Overpass (OpenStreetMap).
//
// Usage policy (https://operations.osmfoundation.org/policies/nominatim/):
// at most one request per second per service, an identifying User-Agent,
// results cached, and "© OpenStreetMap contributors" shown where the data
// is shown. For production volumes, run our own Nominatim/Overpass (#16).
package osm

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/search"
)

const (
	DefaultNominatim = "https://nominatim.openstreetmap.org"
	// DefaultOverpass: public instances, tried in order. overpass-api.de
	// turns away automated use, so it's only the fallback; production runs
	// its own (#16).
	DefaultOverpass = "https://overpass.kumi.systems/api/interpreter,https://overpass-api.de/api/interpreter" //nolint:gosec // URLs, not credentials
	// UserAgent identifies Laamo, as the usage policies ask.
	UserAgent = "Laamo/0.1 (+https://laamo.app/bot)"
	// Attribution must be shown with the data.
	Attribution = "© OpenStreetMap contributors"
)

var (
	// ErrNoPlace: Nominatim doesn't know the place.
	ErrNoPlace = errors.New("osm: place not found")
	// ErrBusy: the public server is overloaded or rate-limits us; try later.
	ErrBusy = errors.New("osm: server busy")
)

// Place is a geocoded place.
type Place struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// Business is one OSM object with a name.
type Business struct {
	OSMURL       string // https://www.openstreetmap.org/node/123
	Name         string
	Address      string
	Website      string
	Phone        string
	Email        string
	OpeningHours string
	DistanceM    float64
}

// Client talks to Nominatim and Overpass. The zero value uses the public
// servers.
type Client struct {
	NominatimURL string
	OverpassURL  string // one or more, comma separated: tried in order while busy
	UserAgent    string
	HTTP         *http.Client

	once      sync.Once
	nomLimit  *search.Limiter
	overLimit *search.Limiter
	mu        sync.Mutex
	cache     map[string]cached
	now       func() time.Time
}

type cached struct {
	val     any
	expires time.Time
}

const (
	placeTTL = 30 * 24 * time.Hour // places don't move
	findTTL  = 24 * time.Hour
	maxCache = 2000
)

func (c *Client) init() {
	c.once.Do(func() {
		c.nomLimit, c.overLimit = search.NewLimiter(1), search.NewLimiter(1)
		c.cache = map[string]cached{}
		if c.now == nil {
			c.now = time.Now
		}
	})
}

func (c *Client) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().After(e.expires) {
		return nil, false
	}
	return e.val, true
}

func (c *Client) put(key string, v any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= maxCache {
		for k, e := range c.cache { // drop the expired ones, or else everything
			if c.now().After(e.expires) {
				delete(c.cache, k)
			}
		}
		if len(c.cache) >= maxCache {
			c.cache = map[string]cached{}
		}
	}
	c.cache[key] = cached{v, c.now().Add(ttl)}
}

func (c *Client) do(ctx context.Context, lim *search.Limiter, req *http.Request) ([]byte, error) {
	if err := lim.Wait(ctx); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", cmp.Or(c.UserAgent, UserAgent))
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 40 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBusy, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return nil, fmt.Errorf("%w: %s", ErrBusy, res.Status)
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("osm: %s: %s", res.Status, strings.TrimSpace(string(body[:min(len(body), 200)])))
	}
	return body, nil
}

// Geocode finds a place by name ("Leipzig", "Berlin-Kreuzberg").
func (c *Client) Geocode(ctx context.Context, q, lang string) (Place, error) {
	c.init()
	q = strings.TrimSpace(q)
	key := "geo|" + lang + "|" + strings.ToLower(q)
	if v, ok := c.get(key); ok {
		return v.(Place), nil
	}
	u := cmp.Or(c.NominatimURL, DefaultNominatim) + "/search?" + url.Values{
		"q": {q}, "format": {"jsonv2"}, "limit": {"1"}, "accept-language": {cmp.Or(lang, "en")},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Place{}, err
	}
	body, err := c.do(ctx, c.nomLimit, req)
	if err != nil {
		return Place{}, err
	}
	var rows []struct {
		Lat  string `json:"lat"`
		Lon  string `json:"lon"`
		Name string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return Place{}, fmt.Errorf("osm: nominatim: %w", err)
	}
	if len(rows) == 0 {
		return Place{}, ErrNoPlace
	}
	lat, err1 := strconv.ParseFloat(rows[0].Lat, 64)
	lon, err2 := strconv.ParseFloat(rows[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return Place{}, fmt.Errorf("osm: nominatim: bad coordinates %q %q", rows[0].Lat, rows[0].Lon)
	}
	p := Place{Name: rows[0].Name, Lat: lat, Lon: lon}
	c.put(key, p, placeTTL)
	return p, nil
}

// Query is what to find around a place.
type Query struct {
	Tags    []Tag  // any of them
	Name    string // or: a name containing these words (when there are no tags)
	Lat     float64
	Lon     float64
	RadiusM int
	Limit   int
}

// Find returns named businesses around a point, nearest first.
func (c *Client) Find(ctx context.Context, q Query) ([]Business, error) {
	c.init()
	ql, err := overpassQL(q)
	if err != nil {
		return nil, err
	}
	if v, ok := c.get("find|" + ql); ok {
		return slices.Clone(v.([]Business)), nil
	}
	var body []byte
	for _, server := range strings.Split(cmp.Or(c.OverpassURL, DefaultOverpass), ",") {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(server),
			strings.NewReader(url.Values{"data": {ql}}.Encode()))
		if rerr != nil {
			return nil, rerr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if body, err = c.do(ctx, c.overLimit, req); err == nil || !errors.Is(err, ErrBusy) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	var res struct {
		Elements []struct {
			Type   string                      `json:"type"`
			ID     int64                       `json:"id"`
			Lat    float64                     `json:"lat"`
			Lon    float64                     `json:"lon"`
			Center *struct{ Lat, Lon float64 } `json:"center"`
			Tags   map[string]string           `json:"tags"`
		} `json:"elements"`
		Remark string `json:"remark"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("osm: overpass: %w", err)
	}
	if strings.Contains(res.Remark, "timed out") || strings.Contains(res.Remark, "out of memory") {
		return nil, fmt.Errorf("%w: %s", ErrBusy, res.Remark)
	}
	var out []Business
	seen := map[string]bool{}
	for _, e := range res.Elements {
		t := e.Tags
		name := strings.TrimSpace(t["name"])
		if name == "" {
			continue
		}
		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}
		b := Business{
			OSMURL:       fmt.Sprintf("https://www.openstreetmap.org/%s/%d", e.Type, e.ID),
			Name:         name,
			Address:      address(t),
			Website:      website(cmp.Or(t["website"], t["contact:website"], t["url"])),
			Phone:        cmp.Or(t["phone"], t["contact:phone"]),
			Email:        email(cmp.Or(t["email"], t["contact:email"])),
			OpeningHours: t["opening_hours"],
			DistanceM:    distance(q.Lat, q.Lon, lat, lon),
		}
		// The same shop as a node and a building: keep one.
		key := strings.ToLower(b.Name + "|" + b.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	slices.SortStableFunc(out, func(a, b Business) int { return cmp.Compare(a.DistanceM, b.DistanceM) })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	c.put("find|"+ql, slices.Clone(out), findTTL)
	return out, nil
}

// overpassQL builds the query. Tags must be valid (ValidTag) and a name is
// escaped, so nothing the user typed can change the query.
func overpassQL(q Query) (string, error) {
	if q.RadiusM <= 0 || q.RadiusM > 50000 {
		return "", fmt.Errorf("osm: radius %d m out of range", q.RadiusM)
	}
	around := fmt.Sprintf("(around:%d,%.6f,%.6f)", q.RadiusM, q.Lat, q.Lon)
	var parts []string
	for _, t := range q.Tags {
		if !ValidTag(t) {
			return "", fmt.Errorf("osm: tag %s=%s not allowed", t.Key, t.Value)
		}
		parts = append(parts, fmt.Sprintf(`nwr["%s"="%s"]["name"]%s;`, t.Key, t.Value, around))
	}
	if len(parts) == 0 {
		name := nameRegex(q.Name)
		if name == "" {
			return "", errors.New("osm: nothing to look for")
		}
		for _, k := range []string{"shop", "amenity", "craft", "office"} {
			parts = append(parts, fmt.Sprintf(`nwr["%s"]["name"~"%s",i]%s;`, k, name, around))
		}
	}
	limit := cmp.Or(q.Limit, 50)
	// More than asked for: some have no name or show up twice.
	return fmt.Sprintf("[out:json][timeout:25];(%s);out center tags %d;", strings.Join(parts, ""), limit*3), nil
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// nameRegex: letters and digits only, words joined by ".*".
func nameRegex(s string) string {
	words := wordRe.FindAllString(s, 5)
	return strings.Join(words, ".*")
}

func address(t map[string]string) string {
	street := strings.TrimSpace(t["addr:street"] + " " + t["addr:housenumber"])
	city := strings.TrimSpace(t["addr:postcode"] + " " + cmp.Or(t["addr:city"], t["addr:suburb"]))
	switch {
	case street != "" && city != "":
		return street + ", " + city
	case street != "":
		return street
	}
	return city
}

func website(s string) string {
	s = strings.TrimSpace(strings.Split(s, ";")[0])
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.Contains(u.Host, ".") {
		return ""
	}
	return s
}

var emailRe = regexp.MustCompile(`^[^@\s<>"]+@[^@\s<>"]+\.[A-Za-z]{2,}$`)

func email(s string) string {
	s = strings.TrimSpace(strings.Split(s, ";")[0])
	if !emailRe.MatchString(s) {
		return ""
	}
	return strings.ToLower(s)
}

// distance in meters (haversine).
func distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

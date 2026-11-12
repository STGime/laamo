package osm

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/STGime/laamo/backend/internal/search"
)

// Geoapify finds businesses with the Geoapify Places API (KEPTAGO LTD,
// Cyprus): OpenStreetMap data, so the same licence and attribution, plus
// "Powered by Geoapify" on the free plan. Free plan: 3000 requests a day,
// 5 a second. Results may be cached and stored.
type Geoapify struct {
	APIKey  string
	BaseURL string // default https://api.geoapify.com
	MapsURL string // default https://maps.geoapify.com
	HTTP    *http.Client

	once  sync.Once
	limit *search.Limiter
	mu    sync.Mutex
	cache map[string]cached
	now   func() time.Time
}

// broad: the categories a name search looks in when the kind is unknown.
var broad = []string{"commercial", "catering", "service", "office", "healthcare", "accommodation", "education", "pet", "childcare"}

// ErrNotConfigured: no API key.
var ErrNotConfigured = errors.New("osm: no Geoapify API key")

func (g *Geoapify) init() {
	g.once.Do(func() {
		g.limit = search.NewLimiter(4) // under the free plan's 5/s
		g.cache = map[string]cached{}
		if g.now == nil {
			g.now = time.Now
		}
	})
}

func (g *Geoapify) get(ctx context.Context, path string, q url.Values, out any) error {
	if g.APIKey == "" {
		return ErrNotConfigured
	}
	g.init()
	key := path + "?" + q.Encode()
	g.mu.Lock()
	if e, ok := g.cache[key]; ok && g.now().Before(e.expires) {
		g.mu.Unlock()
		return json.Unmarshal(e.val.([]byte), out)
	}
	g.mu.Unlock()
	body, err := g.raw(ctx, cmp.Or(g.BaseURL, "https://api.geoapify.com"), path, q)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("geoapify: %w", err)
	}
	ttl := findTTL
	if strings.HasPrefix(path, "/v1/geocode") {
		ttl = placeTTL
	}
	g.remember(key, body, ttl)
	return nil
}

func (g *Geoapify) remember(key string, body []byte, ttl time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cache) >= maxCache {
		g.cache = map[string]cached{}
	}
	if strings.HasPrefix(key, "map|") { // images are big: at most maxMaps of them
		n := 0
		for k := range g.cache {
			if strings.HasPrefix(k, "map|") {
				n++
			}
		}
		if n >= maxMaps {
			maps.DeleteFunc(g.cache, func(k string, _ cached) bool { return strings.HasPrefix(k, "map|") })
		}
	}
	g.cache[key] = cached{body, g.now().Add(ttl)}
}

const maxMaps = 200

// raw makes one request (rate limited) and returns the body.
func (g *Geoapify) raw(ctx context.Context, base, path string, q url.Values) ([]byte, error) {
	if err := g.limit.Wait(ctx); err != nil {
		return nil, err
	}
	q = maps.Clone(q)
	q.Set("apiKey", g.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	hc := g.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the URL holds the key: keep it out of logs
		}
		return nil, fmt.Errorf("%w: geoapify: %w", ErrBusy, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return nil, fmt.Errorf("%w: geoapify: %s", ErrBusy, res.Status)
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("geoapify: the API key was refused (%s)", res.Status)
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("geoapify: %s: %s", res.Status, strings.TrimSpace(string(body[:min(len(body), 200)])))
	}
	return body, nil
}

// StaticMap is a small map (JPEG, 560×200, sharp on phones) with a pin,
// credits printed on it. Maps are kept a week.
func (g *Geoapify) StaticMap(ctx context.Context, lat, lon float64) ([]byte, error) {
	if g.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, fmt.Errorf("osm: bad location %f,%f", lat, lon)
	}
	g.init()
	at := fmt.Sprintf("%.5f,%.5f", lon, lat)
	key := "map|" + at
	g.mu.Lock()
	if e, ok := g.cache[key]; ok && g.now().Before(e.expires) {
		g.mu.Unlock()
		return e.val.([]byte), nil
	}
	g.mu.Unlock()
	body, err := g.raw(ctx, cmp.Or(g.MapsURL, "https://maps.geoapify.com"), "/v1/staticmap", url.Values{
		"style": {"osm-bright-grey"}, "width": {"560"}, "height": {"200"}, "scaleFactor": {"2"}, "zoom": {"16"},
		"center": {"lonlat:" + at}, "marker": {"lonlat:" + at + ";color:#ff3b5c;size:medium"},
	})
	if err != nil {
		return nil, err
	}
	g.remember(key, body, 7*24*time.Hour)
	return body, nil
}

func (g *Geoapify) Geocode(ctx context.Context, q, lang string) (Place, error) {
	var res struct {
		Results []struct {
			Formatted string  `json:"formatted"`
			Lat       float64 `json:"lat"`
			Lon       float64 `json:"lon"`
		} `json:"results"`
	}
	err := g.get(ctx, "/v1/geocode/search", url.Values{"text": {strings.TrimSpace(q)}, "format": {"json"}, "limit": {"1"},
		"lang": {cmp.Or(lang, "en")}}, &res)
	if err != nil {
		return Place{}, err
	}
	if len(res.Results) == 0 {
		return Place{}, ErrNoPlace
	}
	r := res.Results[0]
	return Place{Name: r.Formatted, Lat: r.Lat, Lon: r.Lon}, nil
}

func (g *Geoapify) Find(ctx context.Context, q Query) ([]Business, error) {
	if q.RadiusM <= 0 || q.RadiusM > 50000 {
		return nil, fmt.Errorf("osm: radius %d m out of range", q.RadiusM)
	}
	params := url.Values{
		"filter": {fmt.Sprintf("circle:%.6f,%.6f,%d", q.Lon, q.Lat, q.RadiusM)},
		"bias":   {fmt.Sprintf("proximity:%.6f,%.6f", q.Lon, q.Lat)},
		"limit":  {strconv.Itoa(min(cmp.Or(q.Limit, 50)*2, 200))},
	}
	if len(q.Categories) > 0 {
		params.Set("categories", strings.Join(q.Categories, ","))
	} else {
		name := strings.Join(wordRe.FindAllString(q.Name, 5), " ")
		if name == "" {
			return nil, errors.New("osm: nothing to look for")
		}
		params.Set("categories", strings.Join(broad, ","))
		params.Set("name", name)
	}
	var res struct {
		Features []struct {
			Properties struct {
				Name         string  `json:"name"`
				Street       string  `json:"street"`
				Housenumber  string  `json:"housenumber"`
				Postcode     string  `json:"postcode"`
				City         string  `json:"city"`
				Website      string  `json:"website"`
				OpeningHours string  `json:"opening_hours"`
				Distance     float64 `json:"distance"`
				Lat          float64 `json:"lat"`
				Lon          float64 `json:"lon"`
				Contact      struct {
					Phone string `json:"phone"`
					Email string `json:"email"`
				} `json:"contact"`
				Datasource struct {
					Sourcename string         `json:"sourcename"`
					Raw        map[string]any `json:"raw"`
				} `json:"datasource"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := g.get(ctx, "/v2/places", params, &res); err != nil {
		return nil, err
	}
	var out []Business
	seen := map[string]bool{}
	for _, f := range res.Features {
		p := f.Properties
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		raw := p.Datasource.Raw
		str := func(k string) string {
			switch v := raw[k].(type) {
			case string:
				return v
			case float64:
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
			return ""
		}
		b := Business{
			Name:         name,
			Address:      address(map[string]string{"addr:street": p.Street, "addr:housenumber": p.Housenumber, "addr:postcode": p.Postcode, "addr:city": p.City}),
			Website:      website(cmp.Or(p.Website, str("website"), str("contact:website"))),
			Phone:        cmp.Or(p.Contact.Phone, str("phone"), str("contact:phone")),
			Email:        email(cmp.Or(p.Contact.Email, str("email"), str("contact:email"))),
			OpeningHours: cmp.Or(p.OpeningHours, str("opening_hours")),
			Lat:          p.Lat,
			Lon:          p.Lon,
			DistanceM:    p.Distance,
		}
		if p.Datasource.Sourcename == "openstreetmap" {
			if kind := map[string]string{"n": "node", "w": "way", "r": "relation"}[str("osm_type")]; kind != "" && str("osm_id") != "" {
				b.OSMURL = "https://www.openstreetmap.org/" + kind + "/" + strings.TrimPrefix(str("osm_id"), "-")
			}
		}
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
	return out, nil
}

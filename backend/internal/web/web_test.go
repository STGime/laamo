package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/web"
)

const home = `<!doctype html><html><head><title>Bäckerei Krause – Leipzig</title></head><body>
<nav><a href="/">Start</a> <a href="/kontakt">Kontakt</a> <a href="/impressum">Impressum</a> <a href="https://other.example/x">Partner</a></nav>
<article><h1>Willkommen bei Bäckerei Krause</h1>
<p>Seit 1952 backen wir in Leipzig-Gohlis Brot und Brötchen nach alten Rezepten, mit Sauerteig und viel Zeit.
Jeden Morgen ab 6 Uhr gibt es frische Brötchen, und am Wochenende backen wir zusätzlich Kuchen für die ganze Familie.</p>
<p>Unsere Spezialität ist das Gohliser Landbrot, ein kräftiges Roggenmischbrot mit knuspriger Kruste.</p>
<script>var tracking = "do not show";</script>
</article>
<footer>Bäckerei Krause · <a href="mailto:info@baeckerei-krause.example?subject=Hallo">info@baeckerei-krause.example</a> · <a href="tel:+49341123456">0341 123456</a></footer>
</body></html>`

const impressum = `<html><body><h1>Impressum</h1><p>Bäckerei Krause GmbH<br>Hauptstr. 1, 04155 Leipzig<br>Geschäftsführer: Max Krause</p></body></html>`

func server(t *testing.T, robots string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		if robots == "" {
			http.NotFound(w, nil)
			return
		}
		_, _ = w.Write([]byte(robots))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "LaamoBot/") {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(home))
	})
	mux.HandleFunc("/impressum", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		_, _ = w.Write(latin1(impressum))
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusMovedPermanently) })
	mux.HandleFunc("/menu.pdf", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4"))
	})
	mux.HandleFunc("/private/page", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<p>secret</p>")) })
	mux.HandleFunc("/down", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func latin1(s string) []byte {
	var out []byte
	for _, r := range s {
		out = append(out, byte(r)) //nolint:gosec // test text is Latin-1
	}
	return out
}

func TestReadPage(t *testing.T) {
	s := server(t, "")
	f := &web.Fetcher{AllowPrivate: true}
	p, err := f.Read(context.Background(), s.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != s.URL+"/" {
		t.Errorf("URL after redirect = %q", p.URL)
	}
	if !strings.Contains(p.Title, "Bäckerei Krause") {
		t.Errorf("title %q", p.Title)
	}
	if !strings.Contains(p.Text, "Gohliser Landbrot") || strings.Contains(p.Text, "tracking") {
		t.Errorf("text:\n%s", p.Text)
	}
	if len(p.Emails) != 1 || p.Emails[0] != "info@baeckerei-krause.example" || len(p.Phones) != 1 {
		t.Errorf("emails %v phones %v", p.Emails, p.Phones)
	}
	got := web.ContactPages(p, 2)
	if len(got) != 2 || !strings.HasSuffix(got[0], "/impressum") || !strings.HasSuffix(got[1], "/kontakt") {
		t.Errorf("contact pages %v", got)
	}

	// Latin-1 page, not an "article": the whole text is used.
	p, err = f.Read(context.Background(), s.URL+"/impressum")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Text, "Geschäftsführer: Max Krause") || !strings.Contains(p.Text, "04155 Leipzig") {
		t.Errorf("impressum text:\n%s", p.Text)
	}
}

func TestReadErrors(t *testing.T) {
	s := server(t, "User-agent: *\nDisallow: /private/\n")
	f := &web.Fetcher{AllowPrivate: true}
	ctx := context.Background()
	cases := []struct {
		url       string
		want      error
		transient bool
	}{
		{s.URL + "/private/page", web.ErrRobots, false},
		{s.URL + "/menu.pdf", web.ErrNotPage, false},
		{"ftp://example.com/x", web.ErrBadURL, false},
		{"", web.ErrBadURL, false},
	}
	for _, c := range cases {
		_, err := f.Read(ctx, c.url)
		if !errors.Is(err, c.want) || web.Transient(err) != c.transient {
			t.Errorf("%s: err %v (transient %v), want %v", c.url, err, web.Transient(err), c.want)
		}
	}
	var se *web.StatusError
	if _, err := f.Read(ctx, s.URL+"/nope"); !errors.As(err, &se) || se.Status != 404 || web.Transient(err) {
		t.Errorf("404: %v", err)
	}
	if _, err := f.Read(ctx, s.URL+"/down"); !errors.As(err, &se) || !web.Transient(err) {
		t.Errorf("503: %v", err)
	}
}

func TestRobotsForLaamoBot(t *testing.T) {
	s := server(t, "User-agent: LaamoBot\nDisallow: /\n\nUser-agent: *\nAllow: /\n")
	f := &web.Fetcher{AllowPrivate: true}
	if _, err := f.Read(context.Background(), s.URL+"/"); !errors.Is(err, web.ErrRobots) {
		t.Errorf("err %v, want ErrRobots", err)
	}
}

// Without AllowPrivate, localhost (where httptest listens) is refused,
// for the robots.txt request as well as the page.
func TestPrivateAddressesBlocked(t *testing.T) {
	s := server(t, "")
	f := &web.Fetcher{}
	for _, u := range []string{s.URL + "/", strings.Replace(s.URL, "127.0.0.1", "localhost", 1) + "/"} {
		if _, err := f.Read(context.Background(), u); !errors.Is(err, web.ErrBlocked) {
			t.Errorf("%s: err %v, want ErrBlocked", u, err)
		}
	}
}

func TestPublic(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "2a00:1450::1": true, "127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false,
		"172.16.0.1": false, "169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "::1": false,
		"fe80::1": false, "fd00::1": false, "::ffff:127.0.0.1": false, "::ffff:8.8.8.8": true, "224.0.0.1": false,
		"64:ff9b::7f00:1": false,
	} {
		if got := web.Public(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Public(%s) = %v", ip, got)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"baeckerei-krause.de":             "https://baeckerei-krause.de",
		" http://x.example/a#top ":        "http://x.example/a",
		"https://www.x.example/kontakt?a": "https://www.x.example/kontakt?a",
	} {
		u, err := web.Normalize(in)
		if err != nil || u.String() != want {
			t.Errorf("Normalize(%q) = %v, %v", in, u, err)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "mailto:a@b.c", "https://user:pw@x.example/"} {
		if _, err := web.Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) accepted", bad)
		}
	}
}

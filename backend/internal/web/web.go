// Package web reads public web pages for the Read a webpage brick: safe
// fetching (public addresses only, robots.txt, time and size limits) and
// clean text extraction. Everything it returns is untrusted.
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/temoto/robotstxt"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// UserAgent identifies Laamo to the sites it reads. robots.txt rules for
// "LaamoBot" (or "*") apply.
const UserAgent = "LaamoBot/0.1 (+https://laamo.app/bot)"

const (
	DefaultMaxBytes = 2 << 20 // per page
	DefaultTimeout  = 15 * time.Second
	DefaultMaxText  = 20000 // runes of page text kept
	robotsTTL       = time.Hour
	robotsMaxBytes  = 512 << 10
	maxRedirects    = 5
)

var (
	// ErrBlocked: the address isn't public (localhost, private network, …).
	ErrBlocked = errors.New("this address isn't a public website")
	// ErrRobots: the site's robots.txt asks bots not to read the page.
	ErrRobots = errors.New("the site asks bots not to read this page")
	// ErrNotPage: not an HTML or text page (a PDF, an image, …).
	ErrNotPage = errors.New("this isn't a web page")
	// ErrBadURL: not an http(s) URL.
	ErrBadURL = errors.New("this isn't a web address")
)

// StatusError is a non-2xx answer.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("the site answered HTTP %d", e.Status) }

// Transient reports whether trying again later may help (timeouts, 429,
// 5xx); robots, blocked addresses, 404s and non-pages won't change.
func Transient(err error) bool {
	if err == nil || errors.Is(err, ErrBlocked) || errors.Is(err, ErrRobots) || errors.Is(err, ErrNotPage) || errors.Is(err, ErrBadURL) {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status == http.StatusTooManyRequests || se.Status >= 500
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	return true
}

// Page is a read page.
type Page struct {
	URL   string // after redirects
	Title string
	Text  string // main content as plain text
	// Links on the whole page (navigation and footer included), absolute.
	Links []Link
	// Emails and phone numbers from mailto:/tel: links anywhere on the page
	// (footers often hold them, and the main text drops footers).
	Emails []string
	Phones []string
}

type Link struct{ URL, Text string }

// Fetcher reads pages. The zero value is ready to use.
type Fetcher struct {
	HTTP      *http.Client // nil: a client that only connects to public addresses
	UserAgent string
	MaxBytes  int64
	Timeout   time.Duration
	MaxText   int
	// AllowPrivate turns the public-address check off (tests with httptest).
	AllowPrivate bool
	Now          func() time.Time

	once   sync.Once
	client *http.Client
	mu     sync.Mutex
	robots map[string]robotsEntry
}

type robotsEntry struct {
	data *robotstxt.RobotsData
	at   time.Time
}

func (f *Fetcher) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Fetcher) init() {
	f.once.Do(func() {
		f.robots = map[string]robotsEntry{}
		f.client = f.HTTP
		if f.client == nil {
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			if !f.AllowPrivate {
				dialer.Control = publicOnly
			}
			f.client = &http.Client{Transport: &http.Transport{
				Proxy:                 nil, // a proxy would hide the address check
				DialContext:           dialer.DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       30 * time.Second,
			}}
		}
		inner := f.client.CheckRedirect
		c := *f.client
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if err := checkURL(req.URL); err != nil {
				return err
			}
			if err := f.allowed(req.Context(), req.URL); err != nil {
				return err
			}
			if inner != nil {
				return inner(req, via)
			}
			return nil
		}
		f.client = &c
	})
}

// publicOnly refuses connections to non-public addresses. It runs after DNS
// resolution, for every connection (redirects too), so a hostname that
// resolves to 127.0.0.1 or 10.x is refused as well.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ErrBlocked
	}
	if !Public(ip) {
		return fmt.Errorf("%w (%s)", ErrBlocked, ip)
	}
	return nil
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// Public reports whether ip is a public unicast address.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !cgnat.Contains(ip) && !nat64.Contains(ip) &&
		(!ip.Is4() || ip.As4()[0] != 0)
}

func checkURL(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ErrBadURL
	}
	return nil
}

// Normalize turns "example.com" or "www.example.com/x" into a full URL.
func Normalize(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrBadURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, ErrBadURL
	}
	if err := checkURL(u); err != nil {
		return nil, err
	}
	u.Fragment = ""
	return u, nil
}

// Read fetches and reads one page.
func (f *Fetcher) Read(ctx context.Context, rawURL string) (Page, error) {
	f.init()
	u, err := Normalize(rawURL)
	if err != nil {
		return Page{}, err
	}
	if err := f.allowed(ctx, u); err != nil {
		return Page{}, err
	}
	timeout := f.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := f.get(ctx, u.String(), f.maxBytes(), "text/html,application/xhtml+xml,text/plain;q=0.8")
	if err != nil {
		return Page{}, err
	}
	if res.status < 200 || res.status > 299 {
		return Page{}, &StatusError{Status: res.status}
	}
	ctype, final, body := res.ctype, res.url, res.body
	maxText := f.MaxText
	if maxText <= 0 {
		maxText = DefaultMaxText
	}
	switch {
	case strings.HasPrefix(ctype, "text/plain"):
		return Page{URL: final.String(), Text: clip(cleanText(string(toUTF8(body, ctype))), maxText)}, nil
	case ctype == "" || strings.Contains(ctype, "html"):
		p, err := parse(toUTF8(body, ctype), final)
		if err != nil {
			return Page{}, err
		}
		p.Text = clip(p.Text, maxText)
		return p, nil
	default:
		return Page{}, fmt.Errorf("%w (%s)", ErrNotPage, strings.Split(ctype, ";")[0])
	}
}

func (f *Fetcher) maxBytes() int64 {
	if f.MaxBytes > 0 {
		return f.MaxBytes
	}
	return DefaultMaxBytes
}

type result struct {
	status int
	ctype  string
	url    *url.URL // after redirects
	body   []byte
}

func (f *Fetcher) get(ctx context.Context, u string, limit int64, accept string) (result, error) {
	// u was checked by Normalize (http/https, no credentials); where it
	// connects is checked by publicOnly, on every redirect too.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil) //nolint:gosec // G704: see above
	if err != nil {
		return result{}, ErrBadURL
	}
	ua := f.UserAgent
	if ua == "" {
		ua = UserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "de,en;q=0.8")
	resp, err := f.client.Do(req) //nolint:gosec // G704: see above
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL, it's shown elsewhere
		}
		return result{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	// A page bigger than the limit is cut off, not refused: the start of a
	// page holds what matters.
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return result{}, err
	}
	return result{status: resp.StatusCode, ctype: resp.Header.Get("Content-Type"), url: resp.Request.URL, body: body}, nil
}

// allowed checks robots.txt (cached per host for an hour). A missing
// robots.txt (4xx) allows everything; a 5xx disallows everything for now,
// as search engines do.
func (f *Fetcher) allowed(ctx context.Context, u *url.URL) error {
	key := u.Scheme + "://" + u.Host
	f.mu.Lock()
	e, ok := f.robots[key]
	f.mu.Unlock()
	if !ok || f.now().Sub(e.at) > robotsTTL {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, err := f.get(rctx, key+"/robots.txt", robotsMaxBytes, "text/plain")
		cancel()
		if err != nil {
			if errors.Is(err, ErrBlocked) || errors.Is(err, ErrRobots) {
				return err
			}
			return fmt.Errorf("robots.txt: %w", err)
		}
		data, err := robotstxt.FromStatusAndBytes(res.status, res.body)
		if err != nil {
			data, _ = robotstxt.FromStatusAndBytes(http.StatusNotFound, nil) // unparsable: no rules
		}
		e = robotsEntry{data: data, at: f.now()}
		f.mu.Lock()
		f.robots[key] = e
		f.mu.Unlock()
	}
	ua := f.UserAgent
	if ua == "" {
		ua = UserAgent
	}
	path := u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if !e.data.TestAgent(path, strings.SplitN(ua, "/", 2)[0]) {
		return ErrRobots
	}
	return nil
}

func toUTF8(body []byte, ctype string) []byte {
	if utf8.Valid(body) {
		return body
	}
	r, err := charset.NewReader(bytes.NewReader(body), ctype)
	if err != nil {
		return body
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return body
	}
	return out
}

func parse(body []byte, base *url.URL) (Page, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Page{}, fmt.Errorf("%w: %w", ErrNotPage, err)
	}
	p := Page{URL: base.String()}
	seenLink := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if p.Title == "" {
					p.Title = cleanLine(textOf(n))
				}
			case "a":
				href := strings.TrimSpace(attr(n, "href"))
				switch {
				case strings.HasPrefix(strings.ToLower(href), "mailto:"):
					addr, _, _ := strings.Cut(href[len("mailto:"):], "?")
					if addr, err := url.PathUnescape(addr); err == nil && strings.Contains(addr, "@") && !slices.Contains(p.Emails, addr) {
						p.Emails = append(p.Emails, strings.TrimSpace(addr))
					}
				case strings.HasPrefix(strings.ToLower(href), "tel:"):
					if tel := strings.TrimSpace(href[len("tel:"):]); tel != "" && !slices.Contains(p.Phones, tel) {
						p.Phones = append(p.Phones, tel)
					}
				case href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(strings.ToLower(href), "javascript:"):
					if ref, err := base.Parse(href); err == nil && (ref.Scheme == "http" || ref.Scheme == "https") {
						ref.Fragment = ""
						if s := ref.String(); !seenLink[s] && len(p.Links) < 300 {
							seenLink[s] = true
							p.Links = append(p.Links, Link{URL: s, Text: cleanLine(textOf(n))})
						}
					}
				}
			case "script", "style", "noscript", "template", "svg":
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	art, err := readability.FromDocument(doc, base)
	if err == nil && art.Node != nil {
		var b strings.Builder
		_ = art.RenderText(&b)
		p.Text = cleanText(b.String())
		if t := cleanLine(art.Title()); t != "" {
			p.Title = t
		}
	}
	// Small business pages (a contact page, an Impressum) often aren't
	// "articles"; then use the whole visible text.
	if utf8.RuneCountInString(p.Text) < 200 {
		if body := find(doc, "body"); body != nil {
			if all := cleanText(visibleText(body)); utf8.RuneCountInString(all) > utf8.RuneCountInString(p.Text) {
				p.Text = all
			}
		}
	}
	return p, nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func find(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, tag); f != nil {
			return f
		}
	}
	return nil
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "section": true, "article": true, "header": true, "footer": true, "address": true,
	"table": true, "ul": true, "ol": true, "dd": true, "dt": true, "blockquote": true, "main": true, "aside": true, "nav": true,
}

// visibleText is the text of n with line breaks at block elements.
func visibleText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "noscript", "template", "svg", "head":
				return
			}
			if attr(n, "hidden") != "" || strings.Contains(strings.ReplaceAll(attr(n, "style"), " ", ""), "display:none") {
				return
			}
		}
		block := n.Type == html.ElementNode && blockTags[n.Data]
		if block {
			b.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if block {
			b.WriteByte('\n')
		}
	}
	walk(n)
	return b.String()
}

var spaces = regexp.MustCompile(`[ \t\x{00a0}\r\f\v]+`)

func cleanLine(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

// cleanText collapses spaces and keeps at most one empty line in a row.
func cleanText(s string) string {
	var out []string
	blank := false
	for line := range strings.SplitSeq(s, "\n") {
		line = cleanLine(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max])) + " …"
}

var contactWords = regexp.MustCompile(`(?i)impressum|imprint|kontakt|contact|about|[uü]ber[-_ ]?uns|ueber[-_ ]?uns|team|legal-notice`)

// ContactPages picks up to max links on the same site that look like the
// contact, Impressum or about pages, most useful first.
func ContactPages(p Page, max int) []string {
	base, err := url.Parse(p.URL)
	if err != nil {
		return nil
	}
	host := strings.TrimPrefix(base.Hostname(), "www.")
	rank := func(l Link) int {
		s := strings.ToLower(l.URL + " " + l.Text)
		switch {
		case strings.Contains(s, "impressum") || strings.Contains(s, "imprint") || strings.Contains(s, "legal"):
			return 0
		case strings.Contains(s, "kontakt") || strings.Contains(s, "contact"):
			return 1
		default:
			return 2
		}
	}
	var picks []Link
	for _, l := range p.Links {
		u, err := url.Parse(l.URL)
		if err != nil || strings.TrimPrefix(u.Hostname(), "www.") != host || l.URL == p.URL {
			continue
		}
		if !contactWords.MatchString(u.Path) && !contactWords.MatchString(l.Text) {
			continue
		}
		if slices.ContainsFunc(picks, func(x Link) bool { return x.URL == l.URL }) {
			continue
		}
		picks = append(picks, l)
	}
	slices.SortStableFunc(picks, func(a, b Link) int { return rank(a) - rank(b) })
	var out []string
	for _, l := range picks {
		if len(out) == max {
			break
		}
		out = append(out, l.URL)
	}
	return out
}

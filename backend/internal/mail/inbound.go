package mail

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"
	"unicode"

	_ "github.com/emersion/go-message/charset" // ISO-8859-1, Windows-1252, …
	gomail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-msgauth/dkim"
	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/text/unicode/norm"
)

// MaxInbound is the biggest email a helper accepts (attachments included).
const MaxInbound = 15 << 20

// MaxText is how much of an email's text a run gets.
const MaxText = 20_000

// Message is an incoming email, read for a helper.
type Message struct {
	From        string // address, lower case
	FromName    string
	Subject     string
	Date        time.Time
	MessageID   string
	Text        string // the plain text (or the HTML's text), at most MaxText runes
	Attachments []Attachment
}

// Attachment is a file that came with an email (not stored separately: the
// raw email in Storage holds it).
type Attachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// Parse reads a raw email (RFC 5322).
func Parse(raw []byte) (Message, error) {
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil && r == nil {
		return Message{}, fmt.Errorf("mail: reading the email: %w", err)
	}
	var m Message
	h := r.Header
	if from, err := h.AddressList("From"); err == nil && len(from) > 0 {
		m.From, m.FromName = strings.ToLower(from[0].Address), from[0].Name
	}
	if m.From == "" {
		return Message{}, errors.New("mail: the email has no From address")
	}
	m.Subject, _ = h.Subject()
	m.Date, _ = h.Date()
	m.MessageID, _ = h.MessageID()

	var plain, htmlText string
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A broken part: keep what was read so far.
			break
		}
		switch ph := p.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := ph.ContentType()
			body, _ := io.ReadAll(io.LimitReader(p.Body, 4*MaxText))
			switch {
			case ct == "text/plain" && plain == "":
				plain = string(body)
			case ct == "text/html" && htmlText == "":
				htmlText = HTMLText(string(body))
			}
		case *gomail.AttachmentHeader:
			name, _ := ph.Filename()
			ct, _, _ := ph.ContentType()
			n, _ := io.Copy(io.Discard, p.Body)
			m.Attachments = append(m.Attachments, Attachment{Name: name, Type: ct, Size: int(n)})
		}
	}
	text := plain
	if strings.TrimSpace(text) == "" {
		text = htmlText
	}
	m.Text = cut(clean(text), MaxText)
	return m, nil
}

// HTMLText is the readable text of an HTML email: block elements become
// line breaks, scripts and styles are dropped.
func HTMLText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "head", "title":
				skip++
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "table", "blockquote":
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "head", "title":
				if skip > 0 {
					skip--
				}
			case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "table", "blockquote":
				b.WriteByte('\n')
			}
		}
	}
}

// clean normalises line breaks and spaces and drops control characters.
func clean(s string) string {
	s = norm.NFC.String(strings.ReplaceAll(s, "\r\n", "\n"))
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
	lines := strings.Split(s, "\n")
	out := lines[:0]
	blank := 0
	for _, l := range lines {
		l = strings.TrimRightFunc(strings.Join(strings.Fields(l), " "), unicode.IsSpace)
		if l == "" {
			if blank++; blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// SignedBy reports whether the From address's domain signed the email
// (a valid DKIM signature from the same organisation, e.g. mail.example.com
// for example.com). Only then is From to be trusted: anyone can write any
// From. lookupTXT nil = DNS.
func SignedBy(raw []byte, from string, lookupTXT func(string) ([]string, error)) bool {
	_, domain, ok := strings.Cut(from, "@")
	if !ok {
		return false
	}
	vs, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{LookupTXT: lookupTXT, MaxVerifications: 5})
	if err != nil && !errors.Is(err, dkim.ErrTooManySignatures) {
		return false
	}
	org := orgDomain(domain)
	for _, v := range vs {
		if v.Err == nil && org != "" && orgDomain(v.Domain) == org {
			return true
		}
	}
	return false
}

// orgDomain is a domain's registrable part (mail.example.co.uk → example.co.uk).
func orgDomain(d string) string {
	d = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
	org, err := publicsuffix.EffectiveTLDPlusOne(d)
	if err != nil {
		return ""
	}
	return org
}

// Allowed reports whether from matches a sender list entry: an address
// (anna@example.com) or a whole domain (@example.com or example.com).
// gmail.com and googlemail.com are one mailbox.
func Allowed(from string, senders []string) bool {
	from = sameMailbox(from)
	_, domain, _ := strings.Cut(from, "@")
	for _, s := range senders {
		s = sameMailbox(s)
		switch {
		case s == "":
		case strings.Contains(s, "@") && !strings.HasPrefix(s, "@"):
			if s == from {
				return true
			}
		case strings.TrimPrefix(s, "@") == domain:
			return true
		}
	}
	return false
}

func sameMailbox(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	if local, ok := strings.CutSuffix(a, "googlemail.com"); ok && (local == "" || strings.HasSuffix(local, "@")) {
		a = local + "gmail.com"
	}
	return a
}

// NewAddress makes a helper's address: a readable part from its name and a
// random part, e.g. meetings.x7k2qa@in.laamo.app. The random part keeps
// strangers from guessing it.
func NewAddress(helperName, domain string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(norm.NFKD.String(helperName)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
				b.WriteByte('-')
			}
		}
		if b.Len() >= 20 {
			break
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "helper"
	}
	const letters = "abcdefghijkmnpqrstuvwxyz23456789" // no l, o, 0, 1
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = letters[int(buf[i])%len(letters)]
	}
	return name + "." + string(buf) + "@" + domain
}

// Recipient reads the address a mail was sent to (lower case, without
// "+tags"), as the Worker passes it.
func Recipient(s string) (string, error) {
	a, err := (&mime.WordDecoder{}).DecodeHeader(s)
	if err != nil {
		a = s
	}
	a = strings.ToLower(strings.TrimSpace(strings.Trim(a, "<>")))
	local, domain, ok := strings.Cut(a, "@")
	if !ok || local == "" || domain == "" || strings.ContainsAny(a, " ,;") {
		return "", fmt.Errorf("mail: %q isn't an address", s)
	}
	local, _, _ = strings.Cut(local, "+")
	return local + "@" + domain, nil
}

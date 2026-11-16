package mail_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/STGime/laamo/backend/internal/mail"
)

const plainMail = "From: Anna Schmidt <Anna@Example.com>\r\n" +
	"To: meetings.x7k2qa@in.laamo.app\r\n" +
	"Subject: =?UTF-8?Q?Termin_f=C3=BCr_Donnerstag?=\r\n" +
	"Date: Mon, 16 Nov 2026 09:30:00 +0100\r\n" +
	"Message-ID: <abc123@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=XX\r\n" +
	"\r\n" +
	"--XX\r\n" +
	"Content-Type: multipart/alternative; boundary=YY\r\n" +
	"\r\n" +
	"--YY\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Hallo,\r\n\r\n\r\n\r\nkönnen wir uns Donnerstag treffen?\r\n" +
	"--YY\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>Hallo,</p><p>können wir uns <b>Donnerstag</b> treffen?</p>\r\n" +
	"--YY--\r\n" +
	"--XX\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=\"Angebot.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"JVBERi0xLjQK\r\n" +
	"--XX--\r\n"

func TestParse(t *testing.T) {
	m, err := mail.Parse([]byte(plainMail))
	if err != nil {
		t.Fatal(err)
	}
	if m.From != "anna@example.com" || m.FromName != "Anna Schmidt" || m.Subject != "Termin für Donnerstag" || m.MessageID != "abc123@example.com" {
		t.Fatalf("header: %+v", m)
	}
	if m.Text != "Hallo,\n\nkönnen wir uns Donnerstag treffen?" {
		t.Fatalf("text: %q", m.Text)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "Angebot.pdf" || m.Attachments[0].Size != 9 {
		t.Fatalf("attachments: %+v", m.Attachments)
	}

	// HTML only: its text.
	htmlOnly := "From: a@b.de\r\nSubject: x\r\nContent-Type: text/html\r\n\r\n<html><head><style>p{}</style></head><body><p>Erste Zeile</p><script>evil()</script><div>Zweite&nbsp;Zeile</div></body></html>"
	if m, err := mail.Parse([]byte(htmlOnly)); err != nil || m.Text != "Erste Zeile\n\nZweite Zeile" {
		t.Fatalf("html: %q %v", m.Text, err)
	}
	if _, err := mail.Parse([]byte("Subject: no sender\r\n\r\nhi")); err == nil {
		t.Fatal("no From: no error")
	}
}

func TestAllowed(t *testing.T) {
	for _, c := range []struct {
		from    string
		senders []string
		want    bool
	}{
		{"anna@example.com", []string{"Anna@Example.com"}, true},
		{"ben@example.com", []string{"anna@example.com"}, false},
		{"ben@example.com", []string{"@example.com"}, true},
		{"ben@example.com", []string{"example.com"}, true},
		{"ben@evil-example.com", []string{"example.com"}, false},
		{"anna@example.com", nil, false},
	} {
		if got := mail.Allowed(c.from, c.senders); got != c.want {
			t.Errorf("Allowed(%q, %v) = %v", c.from, c.senders, got)
		}
	}
}

func TestNewAddress(t *testing.T) {
	a := mail.NewAddress("Termine & Meetings – Büro", "in.laamo.app")
	if !regexp.MustCompile(`^termine-meetings-bur\.[a-z2-9]{6}@in\.laamo\.app$`).MatchString(a) {
		t.Fatalf("address %q", a)
	}
	if b := mail.NewAddress("Termine & Meetings – Büro", "in.laamo.app"); a == b {
		t.Fatal("two helpers got the same address")
	}
	if a := mail.NewAddress("😀", "in.laamo.app"); !strings.HasPrefix(a, "helper.") {
		t.Fatalf("no letters: %q", a)
	}
}

func TestRecipient(t *testing.T) {
	if a, err := mail.Recipient("<Meetings.X7K2QA+news@In.Laamo.App>"); err != nil || a != "meetings.x7k2qa@in.laamo.app" {
		t.Fatalf("%q %v", a, err)
	}
	for _, bad := range []string{"", "nobody", "a@b.de, c@d.de"} {
		if _, err := mail.Recipient(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// A signature from the sender's own domain (or a subdomain) counts; one from
// another domain, a changed email or none doesn't.
func TestSignedBy(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	record := "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub)
	lookup := func(name string) ([]string, error) {
		switch name {
		case "sel._domainkey.mail.example.com", "sel._domainkey.other.org":
			return []string{record}, nil
		}
		return nil, errors.New("no such record")
	}
	sign := func(domain, msg string) []byte {
		var out bytes.Buffer
		if err := dkim.Sign(&out, strings.NewReader(msg), &dkim.SignOptions{Domain: domain, Selector: "sel", Signer: priv}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	signed := sign("mail.example.com", plainMail)
	if !mail.SignedBy(signed, "anna@example.com", lookup) {
		t.Fatal("own domain's signature not accepted")
	}
	if mail.SignedBy(sign("other.org", plainMail), "anna@example.com", lookup) {
		t.Fatal("another domain's signature accepted")
	}
	changed := bytes.Replace(signed, []byte("Donnerstag treffen"), []byte("Freitag treffen!!!"), 1)
	if mail.SignedBy(changed, "anna@example.com", lookup) {
		t.Fatal("changed email accepted")
	}
	if mail.SignedBy([]byte(plainMail), "anna@example.com", lookup) {
		t.Fatal("unsigned email accepted")
	}
}

package mail_test

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/mail"
)

const report = `<?xml version="1.0" encoding="UTF-8"?>
<feedback>
  <report_metadata><org_name>mailbox.example</org_name><report_id>42</report_id></report_metadata>
  <policy_published><domain>news.example.com</domain><p>none</p></policy_published>
  <record>
    <row><source_ip>203.0.113.7</source_ip><count>12</count>
      <policy_evaluated><dkim>pass</dkim><spf>pass</spf></policy_evaluated></row>
  </record>
</feedback>`

func gz(t *testing.T, data string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(data))
	_ = w.Close()
	return b.Bytes()
}

func zipped(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(data))
	}
	_ = w.Close()
	return b.Bytes()
}

// b64 wraps base64 at 76 characters, as mail does.
func b64(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\r\n")
		s = s[76:]
	}
	return b.String() + s + "\r\n"
}

func withAttachment(text, name, ctype string, data []byte) string {
	return "From: Reports <noreply@mailbox.example>\r\nTo: watch.x7k2qa@in.laamo.app\r\nSubject: Report domain: news.example.com\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=XX\r\n\r\n" +
		"--XX\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + text + "\r\n" +
		"--XX\r\nContent-Type: " + ctype + "\r\nContent-Disposition: attachment; filename=\"" + name + "\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" + b64(data) + "--XX--\r\n"
}

// A report in a .gz or .zip attachment is read into the email's text.
func TestParseReadsPackedReports(t *testing.T) {
	for name, raw := range map[string]string{
		"gz":  withAttachment("This is an aggregate report.", "mailbox.example!news.example.com!1!2.xml.gz", "application/gzip", gz(t, report)),
		"zip": withAttachment("This is an aggregate report.", "report.zip", "application/zip", zipped(t, map[string]string{"report.xml": report})),
		"xml": withAttachment("This is an aggregate report.", "report.xml", "text/xml", []byte(report)),
	} {
		m, err := mail.Parse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(m.Text, "\n\nThis is an aggregate report.\n\n[Attachment: ") || !strings.Contains(m.Text, "Emails: 12\n") ||
			!strings.Contains(m.Text, "<source_ip>203.0.113.7</source_ip>") || !strings.Contains(m.Text, "<dkim>pass</dkim>") {
			t.Errorf("%s: text %q", name, m.Text)
		}
		if len(m.Attachments) != 1 || m.Attachments[0].Size == 0 {
			t.Errorf("%s: attachments %+v", name, m.Attachments)
		}
	}
}

// Some providers send the file as the whole email, with no text at all.
func TestParseReadsAFileSentAsTheEmail(t *testing.T) {
	raw := "From: noreply@mailbox.example\r\nTo: watch.x7k2qa@in.laamo.app\r\nSubject: Report domain: news.example.com\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: application/zip; name=\"report.zip\"\r\n" +
		"Content-Disposition: attachment; filename=\"report.zip\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		b64(zipped(t, map[string]string{"report.xml": report}))
	m, err := mail.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Text, "\n\n[Attachment: report.zip]\n") || !strings.Contains(m.Text, "<count>12</count>") {
		t.Fatalf("text %q", m.Text)
	}
}

// An email forwarded as an attachment: its text and its report are read.
func TestParseReadsAnAttachedEmail(t *testing.T) {
	inner := withAttachment("This is an aggregate report.", "report.xml.gz", "application/gzip", gz(t, report))
	raw := withAttachment("Fwd, see attached.", "report.eml", "message/rfc822", []byte(inner))
	m, err := mail.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Text, "Subject: Report domain: news.example.com") || !strings.Contains(m.Text, "This is an aggregate report.") {
		t.Fatalf("text %q", m.Text)
	}
	// One level deep only: the attached email's own attachment stays shut.
	if strings.Contains(m.Text, "source_ip") {
		t.Fatalf("opened an attachment inside an attached email: %q", m.Text)
	}
}

// What isn't text stays out, and a file that unpacks to far more than it
// weighs is cut, not swallowed.
func TestParseAttachmentLimits(t *testing.T) {
	for name, raw := range map[string]string{
		"pdf":         withAttachment("See attached.", "offer.pdf", "application/pdf", []byte("%PDF-1.7 \x00\x01\x02")),
		"binary .txt": withAttachment("See attached.", "notes.txt", "text/plain", []byte{0xff, 0xfe, 0x00, 0x41}),
		"zip of exe":  withAttachment("See attached.", "tool.zip", "application/zip", zipped(t, map[string]string{"run.exe": "MZ"})),
		"gz in gz":    withAttachment("See attached.", "a.xml.gz.gz", "application/gzip", gz(t, string(gz(t, report)))),
	} {
		m, err := mail.Parse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if m.Text != "See attached." {
			t.Errorf("%s: text %q", name, m.Text)
		}
	}
	big := withAttachment("See attached.", "big.txt.gz", "application/gzip", gz(t, strings.Repeat("all work and no play\n", 500_000)))
	m, err := mail.Parse([]byte(big))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(m.Text)); n > mail.MaxText+mail.MaxFileText+200 || !strings.Contains(m.Text, "all work and no play") {
		t.Fatalf("%d runes", n)
	}
}

// A DMARC report gets a summary above it with the numbers counted here:
// an email passes when DKIM or SPF passed.
func TestParseSumsUpADMARCReport(t *testing.T) {
	rec := func(ip string, n int, dkim, spf, to string) string {
		return fmt.Sprintf(`<record><row><source_ip>%s</source_ip><count>%d</count><policy_evaluated><disposition>none</disposition>`+
			`<dkim>%s</dkim><spf>%s</spf></policy_evaluated></row><identifiers><envelope_to>%s</envelope_to>`+
			`<header_from>news.example.com</header_from></identifiers></record>`, ip, n, dkim, spf, to)
	}
	xml := `<?xml version="1.0"?><feedback xmlns="urn:ietf:params:xml:ns:dmarc-2.0"><report_metadata><org_name>mailbox.example</org_name>` +
		`<date_range><begin>1795478400</begin><end>1795564799</end></date_range></report_metadata>` +
		`<policy_published><domain>news.example.com</domain><p>none</p></policy_published>` +
		rec("203.0.113.7", 12, "pass", "pass", "a.example") + rec("203.0.113.7", 3, "pass", "fail", "b.example") +
		rec("198.51.100.9", 2, "fail", "fail", "forwarded.example") + `</feedback>`
	intro := strings.Repeat("This is an aggregate report. ", 80) // providers explain at length
	m, err := mail.Parse([]byte(withAttachment(intro, "report.xml.gz", "application/gzip", gz(t, xml))))
	if err != nil {
		t.Fatal(err)
	}
	// On top, before the provider's own words: the AI reads the start.
	if !strings.HasPrefix(m.Text, "DMARC report summary") || strings.Index(m.Text, "Failed sources:") > 600 {
		t.Fatalf("the summary isn't at the top:\n%s", m.Text[:300])
	}
	for _, want := range []string{
		"Reporter: mailbox.example\nDomain: news.example.com\nPeriod: 2026-11-24 to 2026-11-24 (UTC)\nPolicy: none\n",
		"Emails: 17\nPassed DMARC: 15\nFailed DMARC: 2\n",
		"Failed sources: 198.51.100.9 (2: dkim fail, spf fail, sent to forwarded.example)",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("no %q in\n%s", want, m.Text)
		}
	}
	// Other XML is left as it is.
	plain, _ := mail.Parse([]byte(withAttachment("", "feed.xml", "text/xml", []byte("<feedback><note>hello</note></feedback>"))))
	if strings.Contains(plain.Text, "summary") {
		t.Errorf("a summary for %q", plain.Text)
	}
}

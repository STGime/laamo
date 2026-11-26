package mail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Attachments a helper can read as text: plain files (XML, CSV, JSON,
// text), the same inside a .zip or .gz, and an email attached to the email
// (forwarded "as attachment"). Reports that mail providers send (DMARC,
// TLS) come this way: the email itself says nothing, the file everything.
// PDFs and office files are not read.

// MaxFileText is how much text all of an email's attachments together add
// to what a run gets, on top of the email's own text.
const MaxFileText = 20_000

// maxUnpacked: how much of a packed file is unpacked. A small file can
// unpack to gigabytes; nothing past this is read.
const maxUnpacked = 2 << 20

// maxPacked: files read from one .zip.
const maxPacked = 5

var textExt = map[string]bool{".xml": true, ".txt": true, ".csv": true, ".tsv": true, ".json": true, ".log": true, ".md": true}

// readable reports whether an attachment may hold text for the run, going
// by its name and type (before its bytes are kept).
func readable(name, ctype string) bool {
	ext := strings.ToLower(path.Ext(name))
	ct := strings.ToLower(ctype)
	switch {
	case textExt[ext], ext == ".gz", ext == ".zip", ext == ".eml":
		return true
	case ct == "text/plain", ct == "text/csv", ct == "text/xml", ct == "application/xml", ct == "application/json",
		ct == "application/zip", ct == "application/gzip", ct == "application/x-gzip", ct == "message/rfc822":
		return true
	}
	return false
}

// fileText is the text in an attachment, or "" when it holds none we read.
// packed says we are inside an archive or an attached email already: those
// are opened one level deep only.
func fileText(name, ctype string, data []byte, packed bool) string {
	ext := strings.ToLower(path.Ext(name))
	ct := strings.ToLower(ctype)
	switch {
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		if packed {
			return ""
		}
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return ""
		}
		inner, _ := io.ReadAll(io.LimitReader(zr, maxUnpacked))
		return fileText(strings.TrimSuffix(name, path.Ext(name)), "", inner, true)
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if packed {
			return ""
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return ""
		}
		var parts []string
		for i, f := range zr.File {
			if i >= maxPacked {
				break
			}
			if f.FileInfo().IsDir() || !textExt[strings.ToLower(path.Ext(f.Name))] {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			inner, _ := io.ReadAll(io.LimitReader(rc, maxUnpacked))
			_ = rc.Close()
			if t := fileText(f.Name, "", inner, true); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n\n")
	case ext == ".eml" || ct == "message/rfc822":
		if packed {
			return ""
		}
		m, err := parse(data, true)
		if err != nil {
			return ""
		}
		return strings.TrimSpace("Subject: " + m.Subject + "\nFrom: " + m.From + "\n\n" + m.Text)
	case textExt[ext] || strings.HasPrefix(ct, "text/") || ct == "application/xml" || ct == "application/json":
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		if !utf8.Valid(data) {
			return ""
		}
		if ct == "text/html" || ext == ".html" || ext == ".htm" {
			return clean(HTMLText(string(data)))
		}
		return clean(string(data))
	}
	return ""
}

// withFiles adds the attachments' text under the email's own, each with
// its file name, as far as MaxFileText goes. A report's summary goes on
// top of everything: bricks send the AI only the start of a long text, and
// the numbers must be in it.
func withFiles(text string, files []Attachment) string {
	for _, a := range files {
		if sum := dmarcSummary(a.Text); sum != "" && !strings.Contains(text, sum) {
			text = strings.TrimSpace(sum + "\n\n" + text)
		}
	}
	left := MaxFileText
	for _, a := range files {
		if a.Text == "" || left <= 0 {
			continue
		}
		t := cut(a.Text, left)
		left -= utf8.RuneCountInString(t)
		name := a.Name
		if name == "" {
			name = "file"
		}
		text = strings.TrimSpace(text + "\n\n[Attachment: " + name + "]\n" + t)
	}
	return text
}

// Package pdftext reads the text of a PDF: a menu, a price list, a report.
// Only text that is in the file as text: a scanned page is a picture and
// gives nothing.
package pdftext

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const (
	// MaxBytes is the biggest PDF read.
	MaxBytes = 10 << 20
	// MaxPages: pages read from one PDF.
	MaxPages = 20
)

// ErrNoText: the PDF opened, but holds no text (a scan, or only pictures).
var ErrNoText = errors.New("the PDF has no text in it (it may be a scan)")

// Is reports whether data starts like a PDF.
func Is(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00 \t\r\n\xef\xbb\xbf"), []byte("%PDF-"))
}

// Text returns the PDF's text, line by line as it stands on the pages (a
// dish and its price on one line), at most maxRunes. The file comes from
// anywhere: a broken one gives an error, never a crash.
func Text(data []byte, maxRunes int) (text string, err error) {
	if len(data) > MaxBytes {
		return "", fmt.Errorf("the PDF is bigger than %d MB", MaxBytes>>20)
	}
	// The library panics on files it doesn't understand.
	defer func() {
		if x := recover(); x != nil {
			text, err = "", fmt.Errorf("the PDF can't be read: %v", x)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("the PDF can't be read: %w", err)
	}
	var b strings.Builder
	size := 0
	for i := 1; i <= min(r.NumPage(), MaxPages) && size < maxRunes; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue // one unreadable page doesn't spoil the rest
		}
		for _, row := range rows {
			words := slices.Clone(row.Content)
			slices.SortStableFunc(words, func(a, c pdf.Text) int {
				switch {
				case a.X < c.X:
					return -1
				case a.X > c.X:
					return 1
				}
				return 0
			})
			var line strings.Builder
			var end float64
			for j, w := range words {
				// A gap between two pieces is a space; letters set one by
				// one are not.
				if j > 0 && w.X-end > w.FontSize*0.2 && !strings.HasSuffix(line.String(), " ") && !strings.HasPrefix(w.S, " ") {
					line.WriteByte(' ')
				}
				line.WriteString(w.S)
				end = w.X + w.W
			}
			l := strings.Join(strings.Fields(line.String()), " ")
			if l == "" || !utf8.ValidString(l) {
				continue
			}
			b.WriteString(l + "\n")
			size += utf8.RuneCountInString(l) + 1
			if size >= maxRunes {
				break
			}
		}
		b.WriteByte('\n')
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", ErrNoText
	}
	if r := []rune(out); len(r) > maxRunes {
		out = string(r[:maxRunes]) + " …"
	}
	return out, nil
}

// Package pdftexttest makes small PDFs for tests.
package pdftexttest

import (
	"bytes"
	"fmt"
	"strings"
)

// Make writes a one-page PDF with these lines, each as a name on the left
// and (after a tab) a price on the right, the way a menu is set.
func Make(lines ...string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 12 Tf\n")
	for i, l := range lines {
		name, price, _ := strings.Cut(l, "\t")
		y := 760 - 20*i
		fmt.Fprintf(&content, "1 0 0 1 60 %d Tm (%s) Tj\n", y, name)
		if price != "" {
			fmt.Fprintf(&content, "1 0 0 1 420 %d Tm (%s) Tj\n", y, price)
		}
	}
	content.WriteString("ET")
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

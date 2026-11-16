// Command sendmail delivers an email to a helper's address the way the
// Cloudflare Email Worker does (signed with INBOUND_SECRET), for testing
// incoming mail locally:
//
//	go run ./cmd/sendmail termine.x7k2qa@in.laamo.app [mail.eml]
//
// Without a file it sends a short sample email.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/STGime/laamo/backend/internal/api"
)

func main() {
	url := flag.String("api", cmp.Or(os.Getenv("LAAMO_API_URL"), "http://localhost:8787"), "the API's base URL")
	from := flag.String("from", "Anna Schmidt <anna@example.com>", "From of the sample email")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: sendmail [-api URL] [-from ADDR] <helper address> [mail.eml]")
		os.Exit(2)
	}
	secret := os.Getenv("INBOUND_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "INBOUND_SECRET isn't set (in .env)")
		os.Exit(2)
	}
	to := flag.Arg(0)
	raw := []byte("From: " + *from + "\r\nTo: " + to + "\r\nSubject: Termin nächste Woche\r\nDate: " +
		time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <" + strconv.FormatInt(time.Now().UnixNano(), 36) +
		"@example.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHallo,\r\n\r\nkönnen wir uns nächsten Donnerstag um 10 Uhr treffen, " +
		"um über das neue Angebot zu sprechen? Ich bringe die Preisliste mit.\r\n\r\nViele Grüße\r\nAnna\r\n")
	if flag.NArg() > 1 {
		var err error
		if raw, err = os.ReadFile(flag.Arg(1)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	ts := time.Now().Unix()
	// The developer's own API (-api), for testing: not user input.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, *url+"/api/inbound/email", bytes.NewReader(raw)) //nolint:gosec // see above
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Laamo-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Laamo-Signature", hex.EncodeToString(api.InboundSignature(secret, ts, raw)))
	req.Header.Set("X-Laamo-To", to)
	res, err := http.DefaultClient.Do(req) //nolint:gosec // the developer's own API
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	fmt.Printf("%d %s", res.StatusCode, body)
}

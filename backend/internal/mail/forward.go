package mail

import (
	"regexp"
	"strings"
)

// GmailForwarding is who sends Gmail's "confirm forwarding" mail.
const GmailForwarding = "forwarding-noreply@google.com"

var (
	codeInSubject = regexp.MustCompile(`\(#(\d{6,12})\)`)
	codeInText    = regexp.MustCompile(`(?i)code\s*:\s*(\d{6,12})`) // "Confirmation code:", "Bestätigungscode:"
	addressIn     = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
)

// GmailForwardCode reads Gmail's "confirm forwarding" mail: the code to
// type into Gmail and who asked to forward. Only meaningful for a mail from
// GmailForwarding that google.com signed; ok=false if no code is in it.
// ownDomain is the helpers' domain: their addresses aren't the requester.
func GmailForwardCode(m Message, ownDomain string) (code, requester string, ok bool) {
	if c := codeInSubject.FindStringSubmatch(m.Subject); c != nil {
		code = c[1]
	} else if c := codeInText.FindStringSubmatch(m.Text); c != nil {
		code = c[1]
	}
	if code == "" {
		return "", "", false
	}
	for _, src := range []string{m.Subject, m.Text} {
		for _, a := range addressIn.FindAllString(src, -1) {
			if a = strings.ToLower(a); !strings.HasSuffix(a, "@google.com") && !strings.HasSuffix(a, "@"+ownDomain) {
				return code, a, true
			}
		}
	}
	return code, "", true
}

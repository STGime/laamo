package mail

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// A DMARC aggregate report is the file mail providers send a domain's
// owner once a day: how many emails they got in the domain's name, from
// which servers, and whether those passed the domain's checks (RFC 7489,
// appendix C). Its numbers are spread over many records, and adding them
// up is nothing to leave to a language model: Laamo works out a summary
// and puts it above the file.

type dmarcReport struct {
	Meta struct {
		Org   string `xml:"org_name"`
		Range struct {
			Begin int64 `xml:"begin"`
			End   int64 `xml:"end"`
		} `xml:"date_range"`
	} `xml:"report_metadata"`
	Policy struct {
		Domain string `xml:"domain"`
		P      string `xml:"p"`
	} `xml:"policy_published"`
	Records []struct {
		Row struct {
			SourceIP string `xml:"source_ip"`
			Count    int    `xml:"count"`
			Eval     struct {
				DKIM string `xml:"dkim"`
				SPF  string `xml:"spf"`
			} `xml:"policy_evaluated"`
		} `xml:"row"`
		IDs struct {
			EnvelopeTo string `xml:"envelope_to"`
			HeaderFrom string `xml:"header_from"`
		} `xml:"identifiers"`
		Auth struct {
			DKIM []struct {
				Domain string `xml:"domain"`
			} `xml:"dkim"`
		} `xml:"auth_results"`
	} `xml:"record"`
}

// maxFailedSources: failing servers named in the summary.
const maxFailedSources = 10

// dmarcSummary is the summary of a DMARC aggregate report, or "" when the
// text is none. An email passes when DKIM or SPF passed for the domain.
func dmarcSummary(text string) string {
	if !strings.Contains(text, "<feedback") || !strings.Contains(text, "<policy_evaluated>") {
		return ""
	}
	var r dmarcReport
	if err := xml.Unmarshal([]byte(text), &r); err != nil || r.Policy.Domain == "" || len(r.Records) == 0 {
		return ""
	}
	total, failed, foreign := 0, 0, 0
	var sources, strangers []string
	for _, rec := range r.Records {
		n := max(rec.Row.Count, 0)
		total += n
		if strings.EqualFold(rec.Row.Eval.DKIM, "pass") || strings.EqualFold(rec.Row.Eval.SPF, "pass") {
			continue
		}
		failed += n
		if len(sources) < maxFailedSources {
			s := fmt.Sprintf("%s (%d: dkim %s, spf %s", word(rec.Row.SourceIP), n, word(rec.Row.Eval.DKIM), word(rec.Row.Eval.SPF))
			if to := word(rec.IDs.EnvelopeTo); to != "" {
				s += ", sent to " + to
			}
			// Whose email was it? One that carries the domain's own DKIM
			// signature left the domain's servers and was changed on the way
			// (a mailbox passing its mail on, a mailing list). One without
			// it never was the domain's: someone else wrote it.
			own := false
			for _, d := range rec.Auth.DKIM {
				own = own || strings.EqualFold(strings.TrimSpace(d.Domain), strings.TrimSpace(r.Policy.Domain))
			}
			if !own {
				foreign += n
				if len(strangers) < 3 {
					strangers = append(strangers, word(rec.Row.SourceIP))
				}
			}
			if own {
				s += "; it carried the domain's own signature, broken on the way: most likely a mailbox that forwards its mail, not someone faking the domain"
			} else {
				s += "; it carried no signature of the domain: possibly someone else sending in the domain's name"
			}
			sources = append(sources, s+")")
		}
	}
	day := func(unix int64) string {
		if unix <= 0 {
			return "unknown"
		}
		return time.Unix(unix, 0).UTC().Format(time.DateOnly)
	}
	none := "none"
	if len(sources) > 0 {
		none = strings.Join(sources, "; ")
	}
	// The verdict in one line, worked out here: it starts with "All good",
	// "Minor problem" or "Problem" (lists show it as a coloured mark).
	var result string
	switch {
	case failed == 0:
		result = fmt.Sprintf("All good: all %d emails passed.", total)
		if total == 1 {
			result = "All good: the 1 email passed."
		}
	case foreign == 0:
		result = fmt.Sprintf("Minor problem: %d of %d emails failed, but they were the domain's own emails, changed on the way (most likely by a mailbox that forwards its mail). Nothing to do.", failed, total)
	default:
		result = fmt.Sprintf("Problem: %d of %d emails did not come from the domain's own servers (from %s). Someone may be sending in the domain's name.", foreign, total, strings.Join(strangers, ", "))
	}
	return fmt.Sprintf("DMARC report summary (counted by Laamo from the file below)\n"+
		"Result: %s\n"+
		"Reporter: %s\nDomain: %s\nPeriod: %s to %s (UTC)\nPolicy: %s\n"+
		"Emails: %d\nPassed DMARC: %d\nFailed DMARC: %d\nFailed sources: %s",
		result, word(r.Meta.Org), word(r.Policy.Domain), day(r.Meta.Range.Begin), day(r.Meta.Range.End), word(r.Policy.P),
		total, total-failed, failed, none)
}

// word keeps a value from the report to one short piece of text: it is
// someone else's, and it lands in a prompt.
func word(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return cut(s, 80)
}

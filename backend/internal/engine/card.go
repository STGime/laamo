// Package engine runs a helper's stack brick by brick.
//
// A run is a job in the queue. The engine loads the helper version's stack,
// checks it against the catalog, then calls each brick with the pile of
// cards the brick above produced. Each step and its pile are stored
// (run_steps via REST, so the app sees them live; cards via insert_cards).
// Limits are checked before every brick, whether or not the stack has a
// Safety stop. A crashed or retried run resumes at the next brick, from the
// pile of the last finished step.
package engine

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
)

// Trust says where a field's value came from (prompt-injection safety, #22).
// The rule: content a helper reads can change what it writes, never who
// receives it. A recipient must be user_entered, or approved by the user in
// Check first.
const (
	TrustUser      = "user_entered" // typed by the user, from About me or My lists, or edited in Check first
	TrustApproved  = "approved"     // extracted, then approved by the user in Check first
	TrustExtracted = "extracted"    // pulled out by a Think brick, or a structured field of a listing
	TrustUntrusted = "untrusted"    // raw web page or inbound email text
)

// ErrUntrustedTarget: a recipient came from content the helper read and
// the user hasn't approved it.
var ErrUntrustedTarget = errors.New("the recipient came from content the helper read and wasn't approved")

func trustRank(t string) int {
	switch t {
	case TrustUser:
		return 3
	case TrustApproved:
		return 2
	case TrustExtracted:
		return 1
	}
	return 0 // untrusted, unknown or missing
}

// LowestTrust returns the least trusted of ts (untrusted for none).
func LowestTrust(ts ...string) string {
	if len(ts) == 0 {
		return TrustUntrusted
	}
	low := ts[0]
	for _, t := range ts[1:] {
		if trustRank(t) < trustRank(low) {
			low = t
		}
	}
	if trustRank(low) == 0 {
		return TrustUntrusted
	}
	return low
}

// TrustOf returns a field's trust; a field without one is untrusted.
func (c Card) TrustOf(field string) string {
	if t := c.Trust[field]; trustRank(t) > 0 {
		return t
	}
	return TrustUntrusted
}

// CanTarget reports whether a value with this trust may decide who
// receives something.
func CanTarget(trust string) bool { return trust == TrustUser || trust == TrustApproved }

// CheckTargets returns ErrUntrustedTarget if any of the card's target
// fields may not decide a recipient. A missing field is the brick's
// business (it fails as a missing value), not a trust problem.
func (c Card) CheckTargets(fields ...string) error {
	for _, f := range fields {
		if _, ok := c.Fields[f]; ok && !CanTarget(c.TrustOf(f)) {
			return fmt.Errorf("%w (%s is %s)", ErrUntrustedTarget, f, c.TrustOf(f))
		}
	}
	return nil
}

// Approve applies a Check first approval: extracted fields become
// approved, edited fields take the user's value and become user_entered.
// Untrusted fields stay untrusted: raw page text is never a recipient.
func (c *Card) Approve(edits map[string]any) {
	for f := range c.Fields {
		if c.TrustOf(f) == TrustExtracted {
			c.Trust[f] = TrustApproved
		}
	}
	for f, v := range edits {
		c.Set(f, v, TrustUser)
	}
}

// Card is one item in a pile: a search result, a business, an email, a draft.
// Bricks add fields without schema changes.
type Card struct {
	// ID is the card's identity across steps (cards.card_id): an Enrich
	// brick keeps it, so a business found in step 2 is the same card when its
	// email is written in step 7. The engine fills it in for new cards.
	ID     string
	Kind   string
	Fields map[string]any
	Trust  map[string]string // per field
	Source string            // URL it came from, if any
	Parent string            // card it was derived from ("why" trail)
	Kept   *bool             // Let me pick / Keep the good ones
	Score  *float64
	Reason string
}

// Set sets a field with its trust level.
func (c *Card) Set(field string, v any, trust string) {
	if c.Fields == nil {
		c.Fields = map[string]any{}
	}
	if c.Trust == nil {
		c.Trust = map[string]string{}
	}
	c.Fields[field] = v
	c.Trust[field] = trust
}

// Text returns a field as text ("" when missing).
func (c Card) Text(field string) string {
	switch v := c.Fields[field].(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// Has reports whether a field is set and not empty.
func (c Card) Has(field string) bool { return c.Text(field) != "" }

// Clone copies the card, including its field maps.
func (c Card) Clone() Card {
	c.Fields = maps.Clone(c.Fields)
	c.Trust = maps.Clone(c.Trust)
	return c
}

// Config is a brick's settings with catalog defaults filled in.
type Config map[string]any

func (c Config) String(key string) string {
	s, _ := c[key].(string)
	return s
}

func (c Config) Int(key string) int {
	switch v := c[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func (c Config) Float(key string) float64 {
	switch v := c[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func (c Config) Bool(key string) bool {
	b, _ := c[key].(bool)
	return b
}

func (c Config) Strings(key string) []string {
	var out []string
	switch v := c[key].(type) {
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	return out
}

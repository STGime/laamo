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
	"fmt"
	"maps"
	"strconv"
)

// Trust says where a field's value came from (prompt-injection safety, #22).
const (
	TrustUser      = "user_entered" // typed by the user or from About me
	TrustExtracted = "extracted"    // pulled out by a Think brick from read content
	TrustUntrusted = "untrusted"    // raw web page or inbound email text
)

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

package think

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// focus cuts a long text down to the lines about one thing: a menu lists
// forty dishes and the model is sent only the start of a text, so the one
// asked for ("Pizza Salami") would never reach it. Lines are picked by the
// words of about, the best matches first, each with the line before and
// the two after (a price often stands on the next line), and come back in
// their own order. A text that fits, or has no such line, is left alone.
func focus(text, about string, limit int) string {
	if len([]rune(text)) <= limit {
		return text
	}
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(about), func(r rune) bool {
		letter := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127
		return !letter
	}) {
		if len([]rune(w)) >= 3 {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	type hit struct{ line, score int }
	var hits []hit
	for i, l := range lines {
		l = strings.ToLower(l)
		score := 0
		for _, w := range words {
			switch r := []rune(w); {
			case strings.Contains(l, w):
				score += 2
			case len(r) >= 6 && strings.Contains(l, string(r[:4])):
				// "margarita" finds "Margherita": people and menus spell
				// differently.
				score++
			}
		}
		if score > 0 {
			hits = append(hits, hit{i, score})
		}
	}
	if len(hits) == 0 {
		return text
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(b.score, a.score) })
	take := map[int]bool{}
	size := 0
	for _, h := range hits {
		for i := max(h.line-1, 0); i <= min(h.line+2, len(lines)-1); i++ {
			if take[i] {
				continue
			}
			n := len([]rune(lines[i])) + 1
			if size+n > limit {
				continue
			}
			take[i] = true
			size += n
		}
	}
	var out []string
	for i, l := range lines {
		if take[i] && strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

var amountIn = regexp.MustCompile(`\d{1,6}(?:[.,]\d{1,2})?`)

// amount reads the first number in a price as written ("8,50 €", "EUR
// 12.90", "ab 7 €"). ok is false when there is none.
func amount(price string) (float64, bool) {
	m := amountIn.FindString(price)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64)
	return v, err == nil
}

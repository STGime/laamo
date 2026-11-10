package catalog

import (
	"encoding/json"
	"fmt"
)

// normalize turns a config into plain JSON types (float64 numbers, []any),
// so values compare the same whether they came from JSON or Go code.
func normalize(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(v any) any {
	switch v.(type) {
	case nil, string, bool, float64:
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}

func sameValue(a, b any) bool {
	a, b = normalizeValue(a), normalizeValue(b)
	switch x := a.(type) {
	case float64, string, bool:
		return x == b
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// stringList returns the strings in a list value, ignoring anything else.
func stringList(v any) []string {
	list, _ := strictStringList(v)
	return list
}

// strictStringList reports ok=false if v isn't a list of strings.
func strictStringList(v any) ([]string, bool) {
	switch x := normalizeValue(v).(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case nil:
		return nil, true
	}
	return nil, false
}

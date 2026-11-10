package eurobasetest

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// createWorkspace mirrors create_workspace (migrations/0008). It runs with
// f.mu held. The owner foreign key isn't checked (the fake has no users).
func createWorkspace(f *Fake, args map[string]json.RawMessage) (any, error) {
	owner := argString(args, "owner")
	name := strings.TrimSpace(argString(args, "name"))
	tz, locale := "Europe/Berlin", "en"
	if _, ok := args["timezone"]; ok {
		tz = argString(args, "timezone")
	}
	if _, ok := args["locale"]; ok {
		locale = argString(args, "locale")
	}
	bad := func(code, msg string) (any, error) { return nil, &PGError{Code: code, Message: msg} }
	if n := utf8.RuneCountInString(name); n < 1 || n > 80 {
		return bad("22023", "name must be 1 to 80 characters")
	}
	if locale != "en" && locale != "de" {
		return bad("22023", "locale must be en or de")
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" {
		return bad("22023", "unknown timezone "+tz)
	}
	owned := 0
	for _, w := range f.rows["workspaces"] {
		if w["user_id"] == owner {
			owned++
		}
	}
	if owned >= 5 {
		return bad("23514", "you already have 5 workspaces")
	}
	ws, err := f.insert("workspaces", Row{"user_id": owner, "name": name, "timezone": tz, "locale": locale})
	if err != nil {
		return nil, err
	}
	if _, err := f.insert("members", Row{"workspace_id": ws["id"], "user_id": owner, "role": "owner"}); err != nil {
		return nil, err
	}
	if _, err := f.insert("brains", Row{"workspace_id": ws["id"], "user_id": owner}); err != nil {
		return nil, err
	}
	return ws, nil
}

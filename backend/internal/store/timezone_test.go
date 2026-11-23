package store

import "testing"

// The same names as create_workspace (migrations/0020 and its tests in
// migrations/test/workspace_test.sql): a place, or UTC.
func TestValidTimezone(t *testing.T) {
	for _, tz := range []string{"Europe/Berlin", "America/Argentina/Buenos_Aires", "UTC", "Etc/GMT+3", "Asia/Ho_Chi_Minh"} {
		if !ValidTimezone(tz) {
			t.Errorf("%q refused", tz)
		}
	}
	for _, tz := range []string{"", "Local", "Mars/Base", "Europe/Atlantis", "CET", "EST5EDT", "UTC+3", "utc", "GMT",
		"Europe/", "/Berlin", "Europe/Ber lin", "+02:00", "../etc/passwd"} {
		if ValidTimezone(tz) {
			t.Errorf("%q accepted", tz)
		}
	}
}

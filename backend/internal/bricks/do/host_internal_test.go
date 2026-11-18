package do

import "testing"

// A host IDNA lookup refuses still never shows in Unicode.
func TestHostOfNeverUnicode(t *testing.T) {
	for _, raw := range []string{"https://раypal.com/x", "https://pay\u202epal.com", "https://pаypal-ラ.com"} {
		h := hostOf(raw)
		for _, r := range h {
			if r > 0x7f {
				t.Errorf("%q → %q", raw, h)
				break
			}
		}
	}
}

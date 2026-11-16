package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/STGime/laamo/backend/internal/queue"
)

func TestFriendly(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{queue.Permanent(errors.New("this list doesn't exist anymore; pick another in the brick")), "this list doesn't exist anymore; pick another in the brick"},
		{queue.Permanent(fmt.Errorf("couldn't find “Berlin” on the map; try a city, region or country")), "couldn't find “Berlin” on the map; try a city, region or country"},
		{errors.New("eurobase: POST /v1/storage/upload: 1112078 bytes: eurobase: request body over the size limit"), ""},
		{queue.Permanent(fmt.Errorf("couldn't read %s: %w", "https://x.de", errors.New("Get \"https://x.de\": dial tcp: refused"))), ""},
		{queue.Permanent(fmt.Errorf("the list is full: %w", errors.New("pq: too many rows"))), "the list is full"},
		{errors.New("images: fal: status 502"), ""},
	} {
		if got := friendly(c.err); got != c.want {
			t.Errorf("friendly(%q) = %q, want %q", c.err, got, c.want)
		}
	}
	if got := didntWork("Draw a picture", "", "en"); got != "“Draw a picture” didn't work. Please try again later." {
		t.Error(got)
	}
}

package proof

import (
	"errors"
	"testing"
	"time"
)

func TestChecker(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	c := &Checker{Scope: "waitlist", Secret: []byte("k"), Bits: 6, Now: func() time.Time { return now }}
	ch := c.New()
	nonce := Solve(ch)
	if err := c.Check(ch.Token, nonce); err != nil {
		t.Fatalf("good proof: %v", err)
	}
	if err := c.Check(ch.Token, nonce); !errors.Is(err, ErrProof) {
		t.Fatalf("second use: %v", err)
	}

	other := &Checker{Scope: "demo", Secret: []byte("k"), Bits: 6, Now: c.Now}
	ch = other.New()
	if err := c.Check(ch.Token, Solve(ch)); !errors.Is(err, ErrProof) {
		t.Fatalf("token from another form: %v", err)
	}

	ch = c.New()
	nonce = Solve(ch)
	now = now.Add(TTL + time.Second)
	if err := c.Check(ch.Token, nonce); !errors.Is(err, ErrProof) {
		t.Fatalf("old token: %v", err)
	}
	if err := c.Check("1.2.forged", "1"); !errors.Is(err, ErrProof) {
		t.Fatalf("forged: %v", err)
	}
}

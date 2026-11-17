// Package proof is a small proof of work for public forms (the demo, the
// waiting list): the browser asks for a signed challenge and must find a
// nonce so that sha256(token + ":" + nonce) starts with Bits zero bits.
// Cheap for one visitor, expensive for a bot sending thousands.
package proof

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrProof: the token is forged, too old, already used, or the nonce is wrong.
var ErrProof = errors.New("bad or used proof of work")

// TTL: how long a challenge may be used.
const TTL = 10 * time.Minute

// Checker hands out challenges and checks answers. Each token works once.
// Scope keeps tokens for one form from being used on another.
type Checker struct {
	Scope  string
	Secret []byte // empty: a random key (fine for one api instance)
	Bits   int
	Now    func() time.Time

	once sync.Once
	mu   sync.Mutex
	used map[string]time.Time
}

// Challenge is a signed token the browser must find a nonce for.
type Challenge struct {
	Token string
	Bits  int
}

func (c *Checker) init() {
	c.once.Do(func() {
		if len(c.Secret) == 0 {
			c.Secret = make([]byte, 32)
			_, _ = rand.Read(c.Secret)
		}
		c.used = map[string]time.Time{}
	})
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// New returns a challenge valid for TTL and one use.
func (c *Checker) New() Challenge {
	c.init()
	r := make([]byte, 12)
	_, _ = rand.Read(r)
	payload := strconv.FormatInt(c.now().Unix(), 10) + "." + hex.EncodeToString(r)
	return Challenge{Token: payload + "." + c.sign(payload), Bits: c.Bits}
}

func (c *Checker) sign(payload string) string {
	m := hmac.New(sha256.New, c.Secret)
	m.Write([]byte(c.Scope + ":" + payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Check verifies the token and nonce and spends the token.
func (c *Checker) Check(token, nonce string) error {
	c.init()
	i := strings.LastIndexByte(token, '.')
	if i < 0 || len(nonce) == 0 || len(nonce) > 20 || !hmac.Equal([]byte(token[i+1:]), []byte(c.sign(token[:i]))) {
		return ErrProof
	}
	ts, _, _ := strings.Cut(token, ".")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || c.now().Sub(time.Unix(sec, 0)) > TTL {
		return ErrProof
	}
	sum := sha256.Sum256([]byte(token + ":" + nonce))
	if LeadingZeros(sum[:]) < c.Bits {
		return ErrProof
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for t, at := range c.used {
		if c.now().Sub(at) > TTL+time.Minute {
			delete(c.used, t)
		}
	}
	if _, spent := c.used[token]; spent {
		return ErrProof
	}
	c.used[token] = c.now()
	return nil
}

// LeadingZeros counts the zero bits at the start of b.
func LeadingZeros(b []byte) int {
	n := 0
	for _, x := range b {
		if x != 0 {
			return n + bits.LeadingZeros8(x)
		}
		n += 8
	}
	return n
}

// Solve finds a nonce for a challenge (tests and tools; the browser has its own).
func Solve(c Challenge) string {
	for n := 0; ; n++ {
		nonce := strconv.Itoa(n)
		sum := sha256.Sum256([]byte(c.Token + ":" + nonce))
		if LeadingZeros(sum[:]) >= c.Bits {
			return nonce
		}
	}
}

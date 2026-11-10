package crypto_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/crypto"
)

func key() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func ring(t *testing.T, spec, active string) *crypto.Keyring {
	t.Helper()
	k, err := crypto.ParseKeyring(spec, active)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

const webhook = "https://discord.com/api/webhooks/1234567890/abcdefSECRETtoken"

func TestRoundTrip(t *testing.T) {
	k := ring(t, "v1:"+key(), "")
	aad := crypto.AAD("places", "p1", "secret")
	env, err := k.Encrypt([]byte(webhook), aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(env, []byte("SECRET")) {
		t.Fatal("plaintext visible in the envelope")
	}
	got, err := k.Decrypt(env, aad)
	if err != nil || string(got) != webhook {
		t.Fatalf("decrypt: %q %v", got, err)
	}
	again, _ := k.Encrypt([]byte(webhook), aad)
	if bytes.Equal(env, again) {
		t.Fatal("two encryptions of the same secret must differ (fresh data key and nonces)")
	}
	big := bytes.Repeat([]byte("x"), 1<<20)
	env, _ = k.Encrypt(big, aad)
	if got, err := k.Decrypt(env, aad); err != nil || !bytes.Equal(got, big) {
		t.Fatalf("1 MB secret: %v", err)
	}
}

func TestBoundToItsRow(t *testing.T) {
	k := ring(t, "v1:"+key(), "")
	env, _ := k.Encrypt([]byte(webhook), crypto.AAD("places", "p1", "secret"))
	for _, aad := range []string{crypto.AAD("places", "p2", "secret"), crypto.AAD("brains", "p1", "key"), ""} {
		if _, err := k.Decrypt(env, aad); !errors.Is(err, crypto.ErrDecrypt) {
			t.Fatalf("copied to %q: want ErrDecrypt, got %v", aad, err)
		}
	}
}

func TestTamperingIsDetected(t *testing.T) {
	k := ring(t, "v1:"+key(), "")
	aad := "places/p1/secret"
	env, _ := k.Encrypt([]byte(webhook), aad)
	for i := range env {
		bad := bytes.Clone(env)
		bad[i] ^= 0x01
		if _, err := k.Decrypt(bad, aad); err == nil {
			t.Fatalf("flipping byte %d went unnoticed", i)
		}
	}
	for _, short := range [][]byte{nil, {1}, env[:10], env[:len(env)-1]} {
		if _, err := k.Decrypt(short, aad); err == nil {
			t.Fatalf("truncated envelope (%d bytes) accepted", len(short))
		}
	}
}

func TestRotation(t *testing.T) {
	k1, k2 := key(), key()
	aad := "places/p1/secret"

	old := ring(t, "v1:"+k1, "")
	env, _ := old.Encrypt([]byte(webhook), aad)

	// Step 1: add v2 and make it active. Old secrets still decrypt.
	both := ring(t, "v1:"+k1+",v2:"+k2, "")
	if both.Active() != "v2" {
		t.Fatalf("active = %s, want the last listed", both.Active())
	}
	if got, err := both.Decrypt(env, aad); err != nil || string(got) != webhook {
		t.Fatalf("old secret with both keys: %v", err)
	}
	// Step 2: rewrap every stored secret.
	re, err := both.Rewrap(env, aad)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := crypto.KeyID(re); id != "v2" {
		t.Fatalf("rewrapped with %s", id)
	}
	if same, _ := both.Rewrap(re, aad); !bytes.Equal(same, re) {
		t.Fatal("rewrap of an up-to-date envelope must be a no-op")
	}
	// Step 3: retire v1.
	onlyNew := ring(t, "v2:"+k2, "")
	if got, err := onlyNew.Decrypt(re, aad); err != nil || string(got) != webhook {
		t.Fatalf("rewrapped secret after retiring v1: %v", err)
	}
	if _, err := onlyNew.Decrypt(env, aad); !errors.Is(err, crypto.ErrDecrypt) {
		t.Fatalf("a secret still on v1 must fail once v1 is gone: %v", err)
	}
	// Explicit active key (e.g. roll back to v1 for new secrets).
	if r := ring(t, "v1:"+k1+",v2:"+k2, "v1"); r.Active() != "v1" {
		t.Fatalf("explicit active: %s", r.Active())
	}
}

func TestParseKeyringErrors(t *testing.T) {
	good := key()
	cases := map[string][2]string{
		"empty":          {"", ""},
		"no id":          {good, ""},
		"bad id":         {"V 1:" + good, ""},
		"not base64":     {"v1:!!!", ""},
		"short key":      {"v1:" + base64.StdEncoding.EncodeToString([]byte("short")), ""},
		"duplicate id":   {"v1:" + good + ",v1:" + good, ""},
		"unknown active": {"v1:" + good, "v9"},
	}
	for name, c := range cases {
		_, err := crypto.ParseKeyring(c[0], c[1])
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(err.Error(), good) {
			t.Fatalf("%s: error leaks key material: %v", name, err)
		}
	}
	if _, err := crypto.ParseKeyring(" , ", ""); !errors.Is(err, crypto.ErrNoKey) {
		t.Fatalf("blank spec: %v", err)
	}
	var nilRing *crypto.Keyring
	if _, err := nilRing.Encrypt([]byte("x"), "a"); !errors.Is(err, crypto.ErrNoKey) {
		t.Fatalf("nil keyring: %v", err)
	}
}

func TestLast4(t *testing.T) {
	for in, want := range map[string]string{
		webhook:         "…oken",
		"  1234567890 ": "…7890",
		"short":         "…",
		"":              "…",
		"ÄÖÜäöüßxyz":    "…ßxyz",
	} {
		if got := crypto.Last4(in); got != want {
			t.Errorf("Last4(%q) = %q, want %q", in, got, want)
		}
	}
}

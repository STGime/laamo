// Package crypto encrypts secrets at rest: chat webhook URLs, Telegram chat
// ids, Posta keys and (later) users' own LLM keys.
//
// Envelope encryption: every secret gets its own random 256-bit data key
// (AES-256-GCM); the data key is wrapped with a master key (also
// AES-256-GCM). Only the wrapped data key and the ciphertext are stored.
// Master keys are versioned, so rotation means adding a new active key and
// re-wrapping data keys (Rewrap); secrets themselves are never re-encrypted.
//
// Every ciphertext is bound to where it lives (associated data such as
// "places/<id>/secret"), so a ciphertext copied into another row or column
// fails to decrypt.
//
// Plaintext is decrypted only in workers when they need it. The API never
// returns it; it shows Last4 instead.
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

var (
	// ErrDecrypt: the envelope is corrupt, was tampered with, belongs to
	// another row, or its master key is unknown.
	ErrDecrypt = errors.New("crypto: cannot decrypt")
	// ErrNoKey: no master key is configured.
	ErrNoKey = errors.New("crypto: no master key")
)

// format is the envelope layout version.
const format byte = 1

const keySize = 32 // AES-256

var keyIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Keyring holds master keys by id and the id used for new secrets.
type Keyring struct {
	keys   map[string]cipher.AEAD
	active string
}

// ParseKeyring reads master keys from "id:base64key,id:base64key" (each key
// 32 random bytes, e.g. `openssl rand -base64 32`). active names the key for
// new secrets; "" means the last one listed.
func ParseKeyring(spec, active string) (*Keyring, error) {
	k := &Keyring{keys: map[string]cipher.AEAD{}}
	var order []string
	for item := range strings.SplitSeq(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, b64, ok := strings.Cut(item, ":")
		if !ok || !keyIDRe.MatchString(id) {
			return nil, fmt.Errorf("crypto: master key entries look like id:base64 (id = a-z 0-9 _ -), got %q", redact(item))
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(raw) != keySize {
			return nil, fmt.Errorf("crypto: master key %q must be %d bytes, base64", id, keySize)
		}
		if _, dup := k.keys[id]; dup {
			return nil, fmt.Errorf("crypto: master key %q listed twice", id)
		}
		aead, err := newAEAD(raw)
		if err != nil {
			return nil, err
		}
		k.keys[id] = aead
		order = append(order, id)
	}
	if len(order) == 0 {
		return nil, ErrNoKey
	}
	k.active = order[len(order)-1]
	if active != "" {
		if _, ok := k.keys[active]; !ok {
			return nil, fmt.Errorf("crypto: active master key %q is not in the keyring", active)
		}
		k.active = active
	}
	return k, nil
}

func redact(item string) string {
	if id, _, ok := strings.Cut(item, ":"); ok {
		return id + ":…"
	}
	return "…"
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Active is the master key id used for new secrets.
func (k *Keyring) Active() string { return k.active }

// Envelope layout (all big endian):
//
//	1 byte  format (1)
//	1 byte  len(master key id), then the id
//	2 bytes len(wrapped data key), then nonce || sealed data key
//	rest    nonce || sealed plaintext
//
// The data key is sealed with the master key; the plaintext with the data
// key. Both use the caller's associated data plus the master key id.

// Encrypt seals plaintext for the place named by aad (e.g. "places/<id>/secret").
func (k *Keyring) Encrypt(plaintext []byte, aad string) ([]byte, error) {
	if k == nil {
		return nil, ErrNoKey
	}
	dek := make([]byte, keySize)
	_, _ = rand.Read(dek)
	defer clear(dek)
	return k.seal(k.active, dek, plaintext, aad)
}

func (k *Keyring) seal(kid string, dek, plaintext []byte, aad string) ([]byte, error) {
	data, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	ad := associated(kid, aad)
	wrapped := sealWith(k.keys[kid], dek, ad)
	body := sealWith(data, plaintext, ad)
	// Key ids are at most 32 chars (keyIDRe); a wrapped key is 60 bytes.
	if len(kid) > math.MaxUint8 || len(wrapped) > math.MaxUint16 {
		return nil, fmt.Errorf("crypto: envelope header too large")
	}

	var b bytes.Buffer
	b.WriteByte(format)
	b.WriteByte(byte(len(kid))) //nolint:gosec // G115: checked above
	b.WriteString(kid)
	_ = binary.Write(&b, binary.BigEndian, uint16(len(wrapped))) //nolint:gosec // G115: checked above
	b.Write(wrapped)
	b.Write(body)
	return b.Bytes(), nil
}

func sealWith(a cipher.AEAD, plaintext, ad []byte) []byte {
	nonce := make([]byte, a.NonceSize())
	_, _ = rand.Read(nonce)
	return a.Seal(nonce, nonce, plaintext, ad)
}

func openWith(a cipher.AEAD, sealed, ad []byte) ([]byte, error) {
	if len(sealed) < a.NonceSize()+a.Overhead() {
		return nil, ErrDecrypt
	}
	out, err := a.Open(nil, sealed[:a.NonceSize()], sealed[a.NonceSize():], ad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return out, nil
}

func associated(kid, aad string) []byte { return []byte("laamo/v1|" + kid + "|" + aad) }

type envelope struct {
	kid     string
	wrapped []byte
	body    []byte
}

func parse(env []byte) (envelope, error) {
	if len(env) < 4 || env[0] != format {
		return envelope{}, ErrDecrypt
	}
	n := int(env[1])
	if len(env) < 2+n+2 {
		return envelope{}, ErrDecrypt
	}
	kid := string(env[2 : 2+n])
	rest := env[2+n:]
	wl := int(binary.BigEndian.Uint16(rest[:2]))
	rest = rest[2:]
	if len(rest) < wl {
		return envelope{}, ErrDecrypt
	}
	return envelope{kid: kid, wrapped: rest[:wl], body: rest[wl:]}, nil
}

// Decrypt opens an envelope sealed for aad.
func (k *Keyring) Decrypt(env []byte, aad string) ([]byte, error) {
	if k == nil {
		return nil, ErrNoKey
	}
	e, err := parse(env)
	if err != nil {
		return nil, err
	}
	master, ok := k.keys[e.kid]
	if !ok {
		return nil, fmt.Errorf("%w: unknown master key %q", ErrDecrypt, e.kid)
	}
	ad := associated(e.kid, aad)
	dek, err := openWith(master, e.wrapped, ad)
	if err != nil {
		return nil, err
	}
	defer clear(dek)
	data, err := newAEAD(dek)
	if err != nil {
		return nil, ErrDecrypt
	}
	return openWith(data, e.body, ad)
}

// KeyID returns the master key id an envelope was sealed with.
func KeyID(env []byte) (string, error) {
	e, err := parse(env)
	return e.kid, err
}

// Rewrap re-seals an envelope under the active master key (rotation). It
// returns the input unchanged when it already uses the active key.
func (k *Keyring) Rewrap(env []byte, aad string) ([]byte, error) {
	kid, err := KeyID(env)
	if err != nil {
		return nil, err
	}
	if kid == k.active {
		return env, nil
	}
	plaintext, err := k.Decrypt(env, aad)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return k.Encrypt(plaintext, aad)
}

// KeyIDs lists the master key ids, sorted.
func (k *Keyring) KeyIDs() []string {
	ids := make([]string, 0, len(k.keys))
	for id := range k.keys {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Last4 is what the app shows for a stored secret: "…" plus its last 4
// characters, or just "…" for secrets under 8 characters (showing 4 of them
// would give too much away).
func Last4(secret string) string {
	r := []rune(strings.TrimSpace(secret))
	switch {
	case len(r) < 8:
		return "…"
	default:
		return "…" + string(r[len(r)-4:])
	}
}

// AAD builds the associated data for a secret column, e.g.
// AAD("places", id, "secret") → "places/<id>/secret".
func AAD(table, rowID, column string) string { return table + "/" + rowID + "/" + column }

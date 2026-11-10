// Package auth verifies Eurobase end-user access tokens in the Go API.
//
// Laamo users sign in with Eurobase auth (email + password, magic link,
// passkey). Eurobase signs their access tokens with ES256 and publishes the
// project's public key at /v1/auth/.well-known/jwks.json, so the API checks
// tokens locally, without a call to Eurobase per request.
//
// Accepted tokens: alg ES256 only, a known key id, iss "eurobase",
// type "enduser", aud and project_id = this project, not expired.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrInvalidToken: the token is missing, malformed, expired or not ours.
var ErrInvalidToken = errors.New("auth: invalid token")

// User is the signed-in end user.
type User struct {
	ID    string // Eurobase end-user id (users.id), the token's sub
	Email string
}

// Claims are the parts of a Eurobase end-user token Laamo uses.
type claims struct {
	Sub       string          `json:"sub"`
	Email     string          `json:"email"`
	ProjectID string          `json:"project_id"`
	Type      string          `json:"type"`
	Iss       string          `json:"iss"`
	Aud       json.RawMessage `json:"aud"`
	Exp       *int64          `json:"exp"`
	Nbf       *int64          `json:"nbf"`
	Iat       *int64          `json:"iat"`
}

// Verifier checks access tokens against the project's published keys.
type Verifier struct {
	jwksURL   string
	projectID string
	http      *http.Client
	now       func() time.Time
	leeway    time.Duration

	mu        sync.Mutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
	lastTry   time.Time
}

// NewVerifier verifies tokens of the Eurobase project at baseURL
// (e.g. https://laamo.eurobase.app) with id projectID.
func NewVerifier(baseURL, projectID string) *Verifier {
	return &Verifier{
		jwksURL:   strings.TrimRight(baseURL, "/") + "/v1/auth/.well-known/jwks.json",
		projectID: projectID,
		http:      &http.Client{Timeout: 5 * time.Second},
		now:       time.Now,
		leeway:    30 * time.Second,
	}
}

// SetClock replaces time.Now (tests).
func (v *Verifier) SetClock(now func() time.Time) { v.now = now }

const (
	keysMaxAge     = time.Hour        // refetch keys after this
	refetchMinWait = 30 * time.Second // unknown kid: refetch at most this often
)

// Verify checks a compact JWS access token and returns its user.
func (v *Verifier) Verify(ctx context.Context, token string) (User, error) {
	if v.projectID == "" {
		return User{}, fmt.Errorf("%w: verifier has no project id", ErrInvalidToken)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return User{}, fmt.Errorf("%w: not a JWT", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return User{}, fmt.Errorf("%w: header: %w", ErrInvalidToken, err)
	}
	if header.Alg != "ES256" {
		return User{}, fmt.Errorf("%w: alg %q not accepted", ErrInvalidToken, header.Alg)
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return User{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return User{}, fmt.Errorf("%w: bad signature encoding", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return User{}, fmt.Errorf("%w: signature", ErrInvalidToken)
	}

	var c claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return User{}, fmt.Errorf("%w: claims: %w", ErrInvalidToken, err)
	}
	if err := v.checkClaims(c); err != nil {
		return User{}, err
	}
	return User{ID: c.Sub, Email: c.Email}, nil
}

func (v *Verifier) checkClaims(c claims) error {
	now := v.now()
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidToken, msg) }
	switch {
	case c.Exp == nil:
		return bad("no exp")
	case now.After(time.Unix(*c.Exp, 0).Add(v.leeway)):
		return bad("expired")
	case c.Nbf != nil && now.Add(v.leeway).Before(time.Unix(*c.Nbf, 0)):
		return bad("not valid yet")
	case c.Iat != nil && now.Add(v.leeway).Before(time.Unix(*c.Iat, 0)):
		return bad("issued in the future")
	case c.Iss != "eurobase":
		return bad("wrong issuer")
	case c.Type != "enduser":
		return bad("not an end-user token")
	case c.ProjectID != v.projectID:
		return bad("token is for another project")
	case !audIncludes(c.Aud, v.projectID):
		return bad("wrong audience")
	case c.Sub == "":
		return bad("no subject")
	}
	return nil
}

func audIncludes(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return slices.Contains(many, want)
	}
	return false
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// key returns the public key for kid, fetching the JWKS when it's stale or
// the kid is unknown (key rotation), at most every refetchMinWait.
func (v *Verifier) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	stale := v.keys == nil || now.Sub(v.fetchedAt) > keysMaxAge
	_, known := v.keys[kid]
	if (stale || !known) && (v.lastTry.IsZero() || now.Sub(v.lastTry) >= refetchMinWait || v.keys == nil) {
		v.lastTry = now
		keys, err := v.fetch(ctx)
		switch {
		case err == nil:
			v.keys, v.fetchedAt = keys, now
		case v.keys == nil:
			return nil, fmt.Errorf("auth: fetch signing keys: %w", err)
		}
		// On error with keys cached, keep using the old ones.
	}
	if kid == "" && len(v.keys) == 1 {
		for _, k := range v.keys {
			return k, nil
		}
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown key id", ErrInvalidToken)
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	// jwksURL comes from config (EUROBASE_URL), never from a request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil) //nolint:gosec // G704: config URL
	if err != nil {
		return nil, err
	}
	resp, err := v.http.Do(req) //nolint:gosec // G704: config URL
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return parseJWKS(body)
}

func parseJWKS(body []byte) (map[string]*ecdsa.PublicKey, error) {
	var doc struct {
		Keys []struct {
			Kty, Crv, X, Y, Kid, Alg, Use string
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || (k.Alg != "" && k.Alg != "ES256") || (k.Use != "" && k.Use != "sig") {
			continue
		}
		x, errX := base64.RawURLEncoding.DecodeString(k.X)
		y, errY := base64.RawURLEncoding.DecodeString(k.Y)
		if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
			continue
		}
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks: no usable ES256 keys")
	}
	return keys, nil
}

// ---- HTTP ----

type ctxKey struct{}

// WithUser stores the signed-in user in ctx.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the signed-in user, if any.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// TokenVerifier is what the middleware needs (Verifier, or a fake in tests).
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (User, error)
}

// Middleware requires a valid "Authorization: Bearer <token>" on every
// request except the public paths, and puts the user in the context.
// Failures get 401 with the API's {error, code} body.
func Middleware(v TokenVerifier, public func(*http.Request) bool, fail func(w http.ResponseWriter, status int, code, msg string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if public != nil && public(r) {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || strings.TrimSpace(token) == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="laamo"`)
				fail(w, http.StatusUnauthorized, "unauthorized", "sign in first")
				return
			}
			u, err := v.Verify(r.Context(), strings.TrimSpace(token))
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="laamo", error="invalid_token"`)
				if errors.Is(err, ErrInvalidToken) {
					fail(w, http.StatusUnauthorized, "unauthorized", "your session is not valid, sign in again")
				} else {
					fail(w, http.StatusServiceUnavailable, "auth_unavailable", "can't check your session right now")
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/auth"
)

const project = "f852e670-cdc2-4738-856d-be7e1947e2fb"

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

type signer struct {
	kid string
	key *ecdsa.PrivateKey
}

func newSigner(t *testing.T, kid string) signer {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signer{kid: kid, key: k}
}

func (s signer) jwk() map[string]string {
	pub, _ := s.key.PublicKey.Bytes()
	return map[string]string{"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": s.kid,
		"x": base64.RawURLEncoding.EncodeToString(pub[1:33]), "y": base64.RawURLEncoding.EncodeToString(pub[33:])}
}

func b64(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

// sign makes an ES256 token like Eurobase does.
func (s signer) sign(t *testing.T, header, claims map[string]any) string {
	t.Helper()
	input := b64(header) + "." + b64(claims)
	d := sha256.Sum256([]byte(input))
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, d[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func goodClaims() map[string]any {
	return map[string]any{
		"sub": "aaaaaaaa-0000-0000-0000-000000000001", "email": "anna@example.com",
		"project_id": project, "aud": project, "type": "enduser", "iss": "eurobase",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
	}
}

// jwksServer serves the current signers; swap them to rotate keys.
type jwksServer struct {
	*httptest.Server
	mu      sync.Mutex
	signers []signer
	hits    atomic.Int32
	down    atomic.Bool
}

func newJWKS(t *testing.T, signers ...signer) *jwksServer {
	t.Helper()
	j := &jwksServer{signers: signers}
	j.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		j.hits.Add(1)
		if j.down.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		j.mu.Lock()
		var keys []map[string]string
		for _, s := range j.signers {
			keys = append(keys, s.jwk())
		}
		j.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(j.Close)
	return j
}

func verifier(j *jwksServer, clock *time.Time) *auth.Verifier {
	v := auth.NewVerifier(j.URL, project)
	v.SetClock(func() time.Time { return *clock })
	return v
}

func TestVerify(t *testing.T) {
	s := newSigner(t, "k1")
	other := newSigner(t, "k1") // same kid, different key
	j := newJWKS(t, s)
	clock := now
	v := verifier(j, &clock)
	ctx := context.Background()
	hdr := map[string]any{"alg": "ES256", "typ": "JWT", "kid": "k1"}

	with := func(k string, val any) map[string]any {
		c := goodClaims()
		if val == nil {
			delete(c, k)
		} else {
			c[k] = val
		}
		return c
	}
	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", s.sign(t, hdr, goodClaims()), true},
		{"aud as list", s.sign(t, hdr, with("aud", []string{"x", project})), true},
		{"expired", s.sign(t, hdr, with("exp", now.Add(-time.Minute).Unix())), false},
		{"expired within leeway", s.sign(t, hdr, with("exp", now.Add(-10*time.Second).Unix())), true},
		{"no exp", s.sign(t, hdr, with("exp", nil)), false},
		{"issued in the future", s.sign(t, hdr, with("iat", now.Add(time.Hour).Unix())), false},
		{"other project", s.sign(t, hdr, with("project_id", "other")), false},
		{"wrong audience", s.sign(t, hdr, with("aud", "other")), false},
		{"no audience", s.sign(t, hdr, with("aud", nil)), false},
		{"wrong issuer", s.sign(t, hdr, with("iss", "evil")), false},
		{"refresh-type token", s.sign(t, hdr, with("type", "refresh")), false},
		{"no subject", s.sign(t, hdr, with("sub", "")), false},
		{"signed by another key", other.sign(t, hdr, goodClaims()), false},
		{"HS256", s.sign(t, map[string]any{"alg": "HS256", "kid": "k1"}, goodClaims()), false},
		{"alg none", b64(map[string]any{"alg": "none"}) + "." + b64(goodClaims()) + ".", false},
		{"unknown kid", s.sign(t, map[string]any{"alg": "ES256", "kid": "nope"}, goodClaims()), false},
		{"garbage", "not.a.jwt", false},
		{"two parts", "a.b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := v.Verify(ctx, tc.token)
			if tc.ok {
				if err != nil {
					t.Fatalf("want ok, got %v", err)
				}
				if u.ID != "aaaaaaaa-0000-0000-0000-000000000001" || u.Email != "anna@example.com" {
					t.Fatalf("user: %+v", u)
				}
				return
			}
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}

	// Tampering with the claims breaks the signature.
	tok := s.sign(t, hdr, goodClaims())
	parts := strings.Split(tok, ".")
	evil := goodClaims()
	evil["sub"] = "bbbbbbbb-0000-0000-0000-000000000002"
	if _, err := v.Verify(ctx, parts[0]+"."+b64(evil)+"."+parts[2]); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("tampered token accepted: %v", err)
	}
}

func TestKeyRotationAndCaching(t *testing.T) {
	k1, k2 := newSigner(t, "k1"), newSigner(t, "k2")
	j := newJWKS(t, k1)
	clock := now
	v := verifier(j, &clock)
	ctx := context.Background()
	hdr := func(kid string) map[string]any { return map[string]any{"alg": "ES256", "kid": kid} }

	for range 5 {
		if _, err := v.Verify(ctx, k1.sign(t, hdr("k1"), goodClaims())); err != nil {
			t.Fatal(err)
		}
	}
	if j.hits.Load() != 1 {
		t.Fatalf("keys should be fetched once and cached, got %d fetches", j.hits.Load())
	}

	// Eurobase rotates to k2. Right away an unknown kid may refetch once
	// per 30s; just after the first fetch it must not hammer the endpoint.
	j.mu.Lock()
	j.signers = []signer{k1, k2}
	j.mu.Unlock()
	if _, err := v.Verify(ctx, k2.sign(t, hdr("k2"), goodClaims())); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("refetch must wait 30s after the last fetch, got %v", err)
	}
	clock = clock.Add(31 * time.Second)
	if _, err := v.Verify(ctx, k2.sign(t, hdr("k2"), goodClaims())); err != nil {
		t.Fatalf("new key after refetch: %v", err)
	}
	if j.hits.Load() != 2 {
		t.Fatalf("want 2 fetches, got %d", j.hits.Load())
	}

	// JWKS endpoint down: cached keys keep working.
	j.down.Store(true)
	clock = clock.Add(2 * time.Hour)
	if _, err := v.Verify(ctx, k1.sign(t, hdr("k1"), goodClaims())); err == nil {
		t.Fatal("want expired-token error after 2h with the fixed claims")
	}
	c := goodClaims()
	c["exp"] = clock.Add(time.Hour).Unix()
	if _, err := v.Verify(ctx, k1.sign(t, hdr("k1"), c)); err != nil {
		t.Fatalf("cached keys should be used while Eurobase is down: %v", err)
	}
}

func TestMiddleware(t *testing.T) {
	s := newSigner(t, "k1")
	j := newJWKS(t, s)
	clock := now
	v := verifier(j, &clock)

	var gotUser auth.User
	h := auth.Middleware(v,
		func(r *http.Request) bool { return r.URL.Path == "/api/healthz" },
		func(w http.ResponseWriter, status int, code, msg string) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "error": msg})
		},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = auth.UserFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	call := func(path, authz string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		if authz != "" {
			r.Header.Set("Authorization", authz)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	good := s.sign(t, map[string]any{"alg": "ES256", "kid": "k1"}, goodClaims())

	if w := call("/api/healthz", ""); w.Code != http.StatusNoContent {
		t.Fatalf("public path: %d", w.Code)
	}
	if w := call("/api/me", ""); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call("/api/me", "Basic abc"); w.Code != http.StatusUnauthorized {
		t.Fatalf("basic auth: %d", w.Code)
	}
	if w := call("/api/me", "Bearer not.a.jwt"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", w.Code)
	}
	if w := call("/api/me", "Bearer "+good); w.Code != http.StatusNoContent || gotUser.Email != "anna@example.com" {
		t.Fatalf("good token: %d %+v", w.Code, gotUser)
	}

	// Keys can't be fetched at all (cold start, Eurobase down): 503, not 401.
	j2 := newJWKS(t, s)
	j2.down.Store(true)
	h2 := auth.Middleware(verifier(j2, &clock), nil, func(w http.ResponseWriter, status int, _, _ string) { w.WriteHeader(status) })(http.NotFoundHandler())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+good)
	w := httptest.NewRecorder()
	h2.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("keys unavailable: want 503, got %d", w.Code)
	}
}

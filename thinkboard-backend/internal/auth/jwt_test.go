package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func startFakeJWKS(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	set := jwkSet{Keys: []jwk{{
		Kty: "RSA",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}), // 65537
	}}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(set)
	}))
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid, issuer, subject string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestVerifier_Verify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwks := startFakeJWKS(t, key, "test-kid")
	defer jwks.Close()

	v := &Verifier{JWKSURL: jwks.URL, ExpectedIssuer: "https://issuer.example/auth/v1", HTTP: http.DefaultClient, keys: map[string]any{}}

	t.Run("valid token", func(t *testing.T) {
		tok := signToken(t, key, "test-kid", v.ExpectedIssuer, "user-123", time.Now().Add(time.Hour))
		id, err := v.Verify(context.Background(), tok)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if id != "user-123" {
			t.Fatalf("expected user-123, got %s", id)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		tok := signToken(t, key, "test-kid", v.ExpectedIssuer, "user-123", time.Now().Add(-time.Hour))
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Fatal("expected an error for expired token")
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		tok := signToken(t, key, "test-kid", "https://someone-else", "user-123", time.Now().Add(time.Hour))
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Fatal("expected an error for wrong issuer")
		}
	})

	t.Run("unknown kid triggers refetch then fails", func(t *testing.T) {
		otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		tok := signToken(t, otherKey, "other-kid", v.ExpectedIssuer, "user-123", time.Now().Add(time.Hour))
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Fatal("expected an error for unknown kid")
		}
	})
}

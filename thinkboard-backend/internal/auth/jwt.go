package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/golang-jwt/jwt/v5"

	"thinkboard-backend/internal/shared/errs"
	"thinkboard-backend/internal/shared/kernel"
)

// Verifier verifies Supabase-issued JWTs against the project's JWKS (§5.6 jwt.go).
type Verifier struct {
	JWKSURL        string
	ExpectedIssuer string
	HTTP           *http.Client

	mu   sync.RWMutex
	keys map[string]any // kid -> *rsa.PublicKey | *ecdsa.PublicKey
}

// NewVerifier builds a Verifier for the given Supabase project base URL.
func NewVerifier(supabaseURL string) *Verifier {
	return &Verifier{
		JWKSURL:        supabaseURL + "/auth/v1/.well-known/jwks.json",
		ExpectedIssuer: supabaseURL + "/auth/v1",
		HTTP:           http.DefaultClient,
		keys:           map[string]any{},
	}
}

func (v *Verifier) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}

	var set jwkSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]any, len(set.Keys))
	for _, k := range set.Keys {
		pub, err := k.publicKey()
		if err != nil {
			continue // skip keys we don't understand (e.g. a future kty) rather than fail the whole set
		}
		keys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func (v *Verifier) lookupKey(kid string) (any, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok := v.keys[kid]
	return key, ok
}

// keyFunc resolves the signing key for a token, fetching JWKS on first use or on an unknown kid
// (key rotation) — at most one refetch per Verify call.
func (v *Verifier) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if key, ok := v.lookupKey(kid); ok {
			return key, nil
		}
		if err := v.fetchJWKS(ctx); err != nil {
			return nil, err
		}
		if key, ok := v.lookupKey(kid); ok {
			return key, nil
		}
		return nil, fmt.Errorf("unknown jwks kid %q", kid)
	}
}

// Verify checks signature, expiry, and issuer, returning the token's subject as an AuthUserID.
func (v *Verifier) Verify(ctx context.Context, tokenString string) (kernel.AuthUserID, error) {
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, v.keyFunc(ctx),
		jwt.WithValidMethods([]string{"RS256", "ES256"}),
		jwt.WithIssuer(v.ExpectedIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", errs.New(http.StatusUnauthorized, "invalid token")
	}
	return kernel.AuthUserID(claims.Subject), nil
}

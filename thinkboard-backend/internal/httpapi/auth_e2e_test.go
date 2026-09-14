package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"thinkboard-backend/internal/platform/config"
	"thinkboard-backend/internal/platform/postgres"
)

// TestAuthFlowEndToEnd validates the documented auth flow (api/api.json "Supabase Auth" +
// "Gateway" folders, api/openapi.yaml's bearerAuth scheme) against a real local stack: Supabase
// Auth issues a token for admin@gmail.com/password, and the gateway's GET /v1/me resolves it to
// a profile_id. Requires `supabase start` and SUPABASE_ANON_KEY; skips cleanly otherwise, since
// this repo's contract (05-agent-limitation.md) doesn't assume Docker is available everywhere.
func TestAuthFlowEndToEnd(t *testing.T) {
	const email = "admin@gmail.com"
	const password = "password"

	anonKey := os.Getenv("SUPABASE_ANON_KEY")
	if anonKey == "" {
		t.Skip("SUPABASE_ANON_KEY not set; run `supabase start` and export the local anon key to exercise this test")
	}

	cfg := config.Load()
	client := &http.Client{Timeout: 5 * time.Second}

	if _, err := client.Get(cfg.SupabaseURL + "/auth/v1/health"); err != nil {
		t.Skipf("Supabase not reachable at %s (run `supabase start`): %v", cfg.SupabaseURL, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		t.Skipf("could not build postgres pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres not reachable at %s (run `supabase start`): %v", cfg.DatabaseURL, err)
	}

	// Idempotent: ignore the "already registered" case, sign-in is the actual assertion.
	_, _ = supabaseAuthRequest(client, cfg.SupabaseURL+"/auth/v1/signup", anonKey, map[string]any{
		"email":    email,
		"password": password,
	})

	signIn, err := supabaseAuthRequest(client, cfg.SupabaseURL+"/auth/v1/token?grant_type=password", anonKey, map[string]any{
		"email":    email,
		"password": password,
	})
	if err != nil {
		t.Fatalf("sign in request failed: %v", err)
	}
	accessToken, _ := signIn["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("sign in did not return access_token: %v", signIn)
	}

	router := NewRouter(cfg, pool)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/me: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/me response: %v", err)
	}
	if body.ProfileID == "" {
		t.Fatalf("expected non-empty profile_id, got body: %s", w.Body.String())
	}
}

func supabaseAuthRequest(client *http.Client, url, anonKey string, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Authorization", "Bearer "+anonKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return out, &httpStatusError{url: url, status: resp.StatusCode, body: out}
	}
	return out, nil
}

type httpStatusError struct {
	url    string
	status int
	body   map[string]any
}

func (e *httpStatusError) Error() string {
	return "request to " + e.url + " failed"
}

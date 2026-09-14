package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"thinkboard-backend/internal/shared/kernel"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// countingDB wraps fakeDB and counts QueryRow calls, so a test can assert whether the cache
// actually avoided hitting the "database".
type countingDB struct {
	fakeDB
	calls int
}

func (c *countingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.calls++
	return c.fakeDB.QueryRow(ctx, sql, args...)
}

func TestCachedAuthorizer_HitsWithinTTL_MissesAfterExpiry(t *testing.T) {
	db := &countingDB{fakeDB: fakeDB{row: fakeRow{values: []any{true}}}}
	clk := &fakeClock{now: time.Unix(0, 0)}
	cached := NewCachedAuthorizer(NewAuthorizer(db), 60*time.Second, clk)

	profileID := kernel.ProfileID("p1")
	teamID := kernel.TeamID("t1")

	if _, err := cached.IsTeamMember(context.Background(), profileID, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := cached.IsTeamMember(context.Background(), profileID, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.calls != 1 {
		t.Fatalf("expected 1 DB call within TTL, got %d", db.calls)
	}

	clk.now = clk.now.Add(61 * time.Second)
	if _, err := cached.IsTeamMember(context.Background(), profileID, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.calls != 2 {
		t.Fatalf("expected a second DB call after TTL expiry, got %d", db.calls)
	}
}

func TestCachedAuthorizer_DifferentKeysDontShareCache(t *testing.T) {
	db := &countingDB{fakeDB: fakeDB{row: fakeRow{values: []any{true}}}}
	clk := &fakeClock{now: time.Unix(0, 0)}
	cached := NewCachedAuthorizer(NewAuthorizer(db), 60*time.Second, clk)

	if _, err := cached.IsTeamMember(context.Background(), kernel.ProfileID("p1"), kernel.TeamID("t1")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := cached.IsTeamMember(context.Background(), kernel.ProfileID("p2"), kernel.TeamID("t1")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.calls != 2 {
		t.Fatalf("expected a DB call per distinct (profile, team), got %d calls", db.calls)
	}
}

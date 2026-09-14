package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"thinkboard-backend/internal/shared/kernel"
)

// fakeRow is a minimal pgx.Row that scans preset values, standing in for a real query result
// (§8's "Service unit" level: fakes for the collaborators a package needs, no real DB).
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		switch v := d.(type) {
		case *string:
			*v = r.values[i].(string)
		case *bool:
			*v = r.values[i].(bool)
		default:
			return fmt.Errorf("fakeRow: unsupported scan dest %T", d)
		}
	}
	return nil
}

type fakeDB struct {
	row     fakeRow
	lastSQL string
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.lastSQL = sql
	return f.row
}

func TestAuthorizer_ResolveProfileID(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{"profile-1"}}}
	a := NewAuthorizer(db)

	id, err := a.ResolveProfileID(context.Background(), kernel.AuthUserID("auth-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "profile-1" {
		t.Fatalf("expected profile-1, got %s", id)
	}
	if !strings.Contains(db.lastSQL, "profile_identities") {
		t.Fatalf("expected query against profile_identities, got %q", db.lastSQL)
	}
}

func TestAuthorizer_ResolveProfileID_NotFound(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: pgx.ErrNoRows}}
	a := NewAuthorizer(db)

	if _, err := a.ResolveProfileID(context.Background(), kernel.AuthUserID("auth-1")); err == nil {
		t.Fatal("expected an error when no profile_identities row exists")
	}
}

func TestAuthorizer_IsTeamMember(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{true}}}
	a := NewAuthorizer(db)

	ok, err := a.IsTeamMember(context.Background(), kernel.ProfileID("p1"), kernel.TeamID("t1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}
	if !strings.Contains(db.lastSQL, "team_members") {
		t.Fatalf("expected query against team_members, got %q", db.lastSQL)
	}
}

func TestAuthorizer_IsTeamLeader_ChecksRole(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{false}}}
	a := NewAuthorizer(db)

	ok, err := a.IsTeamLeader(context.Background(), kernel.ProfileID("p1"), kernel.TeamID("t1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false")
	}
	if !strings.Contains(db.lastSQL, "role = 'leader'") {
		t.Fatalf("expected role filter in query, got %q", db.lastSQL)
	}
}

func TestAuthorizer_CanAccessSession(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{true}}}
	a := NewAuthorizer(db)

	ok, err := a.CanAccessSession(context.Background(), kernel.ProfileID("p1"), kernel.SessionID("s1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}
	for _, table := range []string{"sessions", "board_columns", "boards", "team_members"} {
		if !strings.Contains(db.lastSQL, table) {
			t.Fatalf("expected query to join %s, got %q", table, db.lastSQL)
		}
	}
}

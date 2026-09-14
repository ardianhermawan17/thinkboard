package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"thinkboard-backend/internal/shared/errs"
	"thinkboard-backend/internal/shared/kernel"
)

// DB is the subset of *pgxpool.Pool the authorizer needs, kept narrow so it's fakeable in tests.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Authorizer is the Go mirror of the schema's RLS security-definer helpers (§5.6): every rule RLS
// enforces on the direct-client path must be re-implemented here for the service-role gateway path.
// Nothing outside this package may query team_members for authorization (03-backend-folder-architecture.md §1).
type Authorizer struct {
	DB DB
}

// NewAuthorizer builds an Authorizer over db (typically a *pgxpool.Pool).
func NewAuthorizer(db DB) *Authorizer {
	return &Authorizer{DB: db}
}

// ResolveProfileID mirrors current_profile_id(): profile_identities.auth_user_id -> profile_id.
func (a *Authorizer) ResolveProfileID(ctx context.Context, authUserID kernel.AuthUserID) (kernel.ProfileID, error) {
	var profileID string
	err := a.DB.QueryRow(ctx,
		`select profile_id from profile_identities where auth_user_id = $1 limit 1`,
		string(authUserID),
	).Scan(&profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errs.New(http.StatusForbidden, "no profile for this account")
		}
		return "", err
	}
	return kernel.ProfileID(profileID), nil
}

// IsTeamMember mirrors is_team_member(target_team).
func (a *Authorizer) IsTeamMember(ctx context.Context, profileID kernel.ProfileID, teamID kernel.TeamID) (bool, error) {
	var ok bool
	err := a.DB.QueryRow(ctx,
		`select exists(select 1 from team_members where team_id = $1 and profile_id = $2)`,
		string(teamID), string(profileID),
	).Scan(&ok)
	return ok, err
}

// IsTeamLeader mirrors is_team_leader(target_team).
func (a *Authorizer) IsTeamLeader(ctx context.Context, profileID kernel.ProfileID, teamID kernel.TeamID) (bool, error) {
	var ok bool
	err := a.DB.QueryRow(ctx,
		`select exists(select 1 from team_members where team_id = $1 and profile_id = $2 and role = 'leader')`,
		string(teamID), string(profileID),
	).Scan(&ok)
	return ok, err
}

// CanAccessSession mirrors can_access_session(target_session): sessions -> board_columns -> boards,
// gated by team membership on the owning board's team.
func (a *Authorizer) CanAccessSession(ctx context.Context, profileID kernel.ProfileID, sessionID kernel.SessionID) (bool, error) {
	var ok bool
	err := a.DB.QueryRow(ctx, `
		select exists (
			select 1
			from sessions s
			join board_columns c on c.id = s.column_id
			join boards b        on b.id = c.board_id
			join team_members tm on tm.team_id = b.team_id
			where s.id = $1 and tm.profile_id = $2
		)`,
		string(sessionID), string(profileID),
	).Scan(&ok)
	return ok, err
}

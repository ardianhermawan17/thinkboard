// Package auth is the RLS replacement for the service-role gateway path (03-backend-folder-architecture.md
// §5.6): every rule RLS enforces on the direct-client path is re-implemented here for the gateway
// path, and nothing else may query team_members for authorization.
//
//   - jwt.go / jwks.go — verify a Supabase-issued JWT against the project's JWKS, returning the
//     subject as a kernel.AuthUserID.
//   - authorizer.go — ResolveProfileID / IsTeamMember / IsTeamLeader / CanAccessSession, an exact
//     Go mirror of the schema's current_profile_id()/is_team_member()/is_team_leader()/can_access_session().
//   - cache.go — a short TTL cache (§5.6: "<=60s") in front of the three membership checks.
package auth

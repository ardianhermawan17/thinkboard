// Package kernel holds shared strong ID types so callers can't pass the wrong id where the
// compiler could catch it (03-backend-folder-architecture.md §5.7).
package kernel

// AuthUserID is the Supabase auth.users.id (the JWT "sub" claim).
type AuthUserID string

// ProfileID is profiles.id — resolved from AuthUserID via profile_identities.
type ProfileID string

// TeamID is teams.id.
type TeamID string

// SessionID is sessions.id.
type SessionID string

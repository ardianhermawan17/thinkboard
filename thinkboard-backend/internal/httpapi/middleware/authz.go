package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"thinkboard-backend/internal/shared/errs"
	"thinkboard-backend/internal/shared/kernel"
)

// teamMembershipChecker is the subset of auth.Authorizer/CachedAuthorizer this middleware needs.
type teamMembershipChecker interface {
	IsTeamMember(ctx context.Context, profileID kernel.ProfileID, teamID kernel.TeamID) (bool, error)
}

// sessionAccessChecker is the subset of auth.Authorizer/CachedAuthorizer this middleware needs.
type sessionAccessChecker interface {
	CanAccessSession(ctx context.Context, profileID kernel.ProfileID, sessionID kernel.SessionID) (bool, error)
}

// RequireTeamMember 403s unless the ProfileID Auth() put on the context belongs to the team named
// by the :paramName URL param. Must run after Auth().
func RequireTeamMember(checker teamMembershipChecker, paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		profileID, ok := ProfileIDFrom(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		teamID := kernel.TeamID(c.Param(paramName))
		isMember, err := checker.IsTeamMember(c.Request.Context(), profileID, teamID)
		if err != nil {
			errs.Fail(c, err)
			c.Abort()
			return
		}
		if !isMember {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a team member"})
			return
		}
		c.Next()
	}
}

// RequireSessionAccess 403s unless the ProfileID Auth() put on the context can access the session
// named by the :paramName URL param (schema's can_access_session). Must run after Auth().
func RequireSessionAccess(checker sessionAccessChecker, paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		profileID, ok := ProfileIDFrom(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		sessionID := kernel.SessionID(c.Param(paramName))
		canAccess, err := checker.CanAccessSession(c.Request.Context(), profileID, sessionID)
		if err != nil {
			errs.Fail(c, err)
			c.Abort()
			return
		}
		if !canAccess {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cannot access session"})
			return
		}
		c.Next()
	}
}

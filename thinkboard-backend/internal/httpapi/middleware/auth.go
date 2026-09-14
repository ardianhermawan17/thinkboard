package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"thinkboard-backend/internal/shared/errs"
	"thinkboard-backend/internal/shared/kernel"
)

// contextProfileID is the gin.Context key Auth() stores the resolved profile id under.
const contextProfileID = "profile_id"

// jwtVerifier is the subset of auth.Verifier this middleware needs.
type jwtVerifier interface {
	Verify(ctx context.Context, tokenString string) (kernel.AuthUserID, error)
}

// profileResolver is the subset of auth.Authorizer this middleware needs.
type profileResolver interface {
	ResolveProfileID(ctx context.Context, authUserID kernel.AuthUserID) (kernel.ProfileID, error)
}

// Auth verifies the request's Bearer JWT and stores the resolved ProfileID on the context (§5.6,
// §6). It is the gateway's replacement for RLS's auth.uid() on every route it guards.
func Auth(verifier jwtVerifier, resolver profileResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		authUserID, err := verifier.Verify(c.Request.Context(), token)
		if err != nil {
			errs.Fail(c, err)
			c.Abort()
			return
		}

		profileID, err := resolver.ResolveProfileID(c.Request.Context(), authUserID)
		if err != nil {
			errs.Fail(c, err)
			c.Abort()
			return
		}

		c.Set(contextProfileID, profileID)
		c.Next()
	}
}

// ProfileIDFrom reads the ProfileID an Auth() middleware stored on the context.
func ProfileIDFrom(c *gin.Context) (kernel.ProfileID, bool) {
	v, ok := c.Get(contextProfileID)
	if !ok {
		return "", false
	}
	id, ok := v.(kernel.ProfileID)
	return id, ok
}

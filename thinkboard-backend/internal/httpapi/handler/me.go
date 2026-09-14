package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thinkboard-backend/internal/httpapi/middleware"
)

// MeHandler proves the auth chain end to end: Auth() middleware -> ProfileID -> here.
type MeHandler struct{}

// Me returns the caller's resolved profile id.
func (h *MeHandler) Me(c *gin.Context) {
	profileID, ok := middleware.ProfileIDFrom(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile_id": profileID})
}

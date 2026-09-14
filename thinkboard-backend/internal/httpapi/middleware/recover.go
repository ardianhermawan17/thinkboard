package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recover turns a panic into a structured 500 — never a stack trace to the client (§6).
func Recover() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	})
}

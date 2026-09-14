package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thinkboard-backend/internal/shared/clock"
)

// HealthHandler serves liveness checks.
type HealthHandler struct {
	Clock clock.Clock
}

// Health responds 200 with the current server time — proof the process is up and answering.
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   h.Clock.Now().UTC(),
	})
}

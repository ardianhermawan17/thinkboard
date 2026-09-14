package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderRequestID is the response header carrying the per-request correlation id.
const HeaderRequestID = "X-Request-Id"

// RequestID stamps every request/response with a correlation id for log/trace joining.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}

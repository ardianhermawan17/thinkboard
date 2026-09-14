// Package errs maps domain errors to HTTP responses, defined once for httpapi to use (§5.7).
package errs

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HTTPError is a domain error carrying its intended HTTP status.
type HTTPError struct {
	Status  int
	Message string
	Err     error
}

func (e *HTTPError) Error() string { return e.Message }
func (e *HTTPError) Unwrap() error { return e.Err }

// Invalid wraps a request-binding error as 400.
func Invalid(err error) error {
	return &HTTPError{Status: http.StatusBadRequest, Message: "invalid request", Err: err}
}

// New builds an HTTPError with an explicit status, for domain code that knows the right response.
func New(status int, message string) error {
	return &HTTPError{Status: status, Message: message}
}

// Fail writes the appropriate JSON error response for err, defaulting to 500.
func Fail(c *gin.Context, err error) {
	var he *HTTPError
	if errors.As(err, &he) {
		c.JSON(he.Status, gin.H{"error": he.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

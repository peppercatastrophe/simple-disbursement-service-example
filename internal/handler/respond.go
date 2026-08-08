package handler

import (
	"errors"
	"net/http"

	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
)

// RespondError maps a service error to an HTTP status and JSON body.
func RespondError(c *gin.Context, err error) {
	var status int
	message := err.Error()
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		status = http.StatusUnauthorized
	case errors.Is(err, service.ErrInvalidToken):
		status = http.StatusUnauthorized
	case errors.Is(err, service.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, service.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrAlreadyTransitioned):
		status = http.StatusConflict
	default:
		status = http.StatusBadRequest
	}
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": http.StatusText(status), "message": message},
	})
}

// RespondOK writes the standard success envelope.
func RespondOK(c *gin.Context, data any, meta any) {
	body := gin.H{"success": true, "data": data}
	if meta != nil {
		body["meta"] = meta
	}
	c.JSON(http.StatusOK, body)
}

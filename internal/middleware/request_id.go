package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// RequestID generates a per-request UUID, echoes it as X-Request-ID, and
// attaches it to the context so downstream logs share one request id.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.NewString()
		c.Set(CtxRequestID, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// StructuredLogger emits one JSON log line per request with propagated fields.
func StructuredLogger(log *zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		requestID, _ := c.Get(CtxRequestID)
		username, _ := c.Get(CtxUsername)
		entry := log.Info().
			Str("request_id", str(requestID)).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status_code", c.Writer.Status()).
			Int64("latency_ms", time.Since(start).Milliseconds())
		if u, ok := username.(string); ok && u != "" {
			entry.Str("user", u)
		}
		entry.Msg("request")
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
)

// Context keys for per-request state.
const (
	CtxUserID    = "user_id"
	CtxUsername  = "username"
	CtxUserRole  = "user_role"
	CtxRequestID = "request_id"
)

// CurrentUser reconstructs the authenticated actor from the request context.
// Use this instead of reading context keys ad hoc; role is stored as model.Role.
func CurrentUser(c *gin.Context) *model.User {
	role, _ := c.Get(CtxUserRole)
	return &model.User{
		ID:       c.GetInt64(CtxUserID),
		Username: c.GetString(CtxUsername),
		Role:     role.(model.Role),
	}
}

// Authenticate enforces a valid Bearer access token and injects the caller.
func Authenticate(tm *service.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "missing bearer token"})
			return
		}
		claims, err := tm.ParseAccess(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "invalid or expired token"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxUserRole, claims.Role)
		c.Next()
	}
}

// RequireRoles rejects callers whose role is not in the allowed set.
func RequireRoles(roles ...model.Role) gin.HandlerFunc {
	allowed := make(map[model.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		role, _ := c.Get(CtxUserRole)
		r, ok := role.(model.Role)
		if !ok || !allowed[r] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "forbidden"})
			return
		}
		c.Next()
	}
}

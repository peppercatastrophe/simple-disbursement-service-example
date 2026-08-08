package handler

import (
	"net/http"

	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler exposes login/refresh/logout endpoints.
type AuthHandler struct {
	users *service.UserService
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(users *service.UserService) *AuthHandler {
	return &AuthHandler{users: users}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Login authenticates and returns a token pair.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, err)
		return
	}
	pair, err := h.users.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pair})
}

// Refresh swaps a refresh token for a new pair.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, err)
		return
	}
	pair, err := h.users.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pair})
}

// Logout invalidates the presented refresh token.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, err)
		return
	}
	if err := h.users.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
}

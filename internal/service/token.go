package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// TokenManager issues and validates JWT access/refresh token pairs.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	// refreshStore persists refresh-token hashes (see ARCHITECTURE.md).
	refreshStore RefreshTokenStore
	// users resolves a refresh token to its owning user.
	users repository.UserRepository
}

// TokenPair is an access + refresh token set.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Claims is the JWT payload carried in access tokens.
type Claims struct {
	UserID   int64      `json:"uid"`
	Username string     `json:"username"`
	Role     model.Role `json:"role"`
	jwt.RegisteredClaims
}

// RefreshTokenStore abstracts refresh-token persistence.
type RefreshTokenStore interface {
	Save(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	IsValid(ctx context.Context, tokenHash string) (int64, error)
	Revoke(ctx context.Context, tokenHash string) error
}

// NewTokenManager builds a TokenManager.
func NewTokenManager(secret []byte, accessTTL, refreshTTL time.Duration, store RefreshTokenStore, users repository.UserRepository) *TokenManager {
	return &TokenManager{secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL, refreshStore: store, users: users}
}

// IssuePair mints an access token and a persisted refresh token for a user.
func (m *TokenManager) IssuePair(ctx context.Context, u *model.User) (*TokenPair, error) {
	access, err := m.sign(u, time.Now().Add(m.accessTTL))
	if err != nil {
		return nil, err
	}
	refresh := uuid.NewString()
	expiresAt := time.Now().Add(m.refreshTTL)
	if err := m.refreshStore.Save(ctx, u.ID, hash(refresh), expiresAt); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.accessTTL.Seconds()),
	}, nil
}

// Refresh validates a refresh token and issues a new pair.
func (m *TokenManager) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	userID, err := m.refreshStore.IsValid(ctx, hash(refreshToken))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	u, err := m.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Invalidate the used refresh token (rotation), then issue a fresh pair.
	if err := m.refreshStore.Revoke(ctx, hash(refreshToken)); err != nil {
		return nil, err
	}
	return m.IssuePair(ctx, u)
}

// Revoke invalidates a refresh token.
func (m *TokenManager) Revoke(ctx context.Context, refreshToken string) error {
	return m.refreshStore.Revoke(ctx, hash(refreshToken))
}

// ParseAccess validates an access token and returns its claims.
func (m *TokenManager) ParseAccess(tokenString string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (m *TokenManager) sign(u *model.User, expiresAt time.Time) (string, error) {
	claims := &Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.Username,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// hash derives a stable, non-reversible identifier for a token value.
func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// verifyPassword compares a plaintext password to its bcrypt hash.
func verifyPassword(hashStr, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashStr), []byte(password)) == nil
}

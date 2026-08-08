package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// RefreshToken is a persisted refresh-token hash (raw token never stored).
type RefreshToken struct {
	ID        int64      `gorm:"primaryKey"`
	UserID    int64      `gorm:"index;not null"`
	TokenHash string     `gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time  `gorm:"index;not null"`
	RevokedAt *time.Time `gorm:"index"`
	CreatedAt time.Time
}

// TableName overrides GORM's pluralization.
func (RefreshToken) TableName() string { return "refresh_tokens" }

// RefreshTokenRepo is the GORM-backed refresh-token store.
type RefreshTokenRepo struct{ db *gorm.DB }

// NewRefreshTokenRepo builds a RefreshTokenRepo.
func NewRefreshTokenRepo(db *gorm.DB) *RefreshTokenRepo { return &RefreshTokenRepo{db: db} }

// Save persists a new refresh token.
func (r *RefreshTokenRepo) Save(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Create(&RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}).Error
}

// IsValid returns the owning user id if the token is unrevoked and unexpired.
func (r *RefreshTokenRepo) IsValid(ctx context.Context, tokenHash string) (int64, error) {
	var t RefreshToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, time.Now()).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	return t.UserID, err
}

// Revoke invalidates a refresh token.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, tokenHash string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Update("revoked_at", now).Error
}

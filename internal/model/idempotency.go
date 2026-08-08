package model

import "time"

// IdempotencyKey claims a POST /disbursements request so a retry with the same
// key returns the original response instead of creating a duplicate.
type IdempotencyKey struct {
	ID             int64     `gorm:"primaryKey"`
	IdempotencyKey string    `gorm:"uniqueIndex;not null"`
	DisbursementID *int64    `gorm:"index"`    // null until the winner commits
	Status         string    `gorm:"not null"` // in_progress | committed
	CreatedAt      time.Time `gorm:"not null"`
}

// TableName overrides GORM's pluralization.
func (IdempotencyKey) TableName() string { return "idempotency_keys" }

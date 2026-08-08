package repository

import (
	"context"
	"fmt"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditLogRepository defines data access for audit logs.
type AuditLogRepository interface {
	// Create persists an audit entry. Implementations MUST NOT fail the
	// caller on error — audit is best-effort and non-blocking.
	Create(ctx context.Context, entry *model.AuditLog) error
	List(ctx context.Context, filter AuditFilter) ([]model.AuditLog, int64, error)
}

// AuditFilter carries pagination and filtering parameters.
type AuditFilter struct {
	Page     int
	Limit    int
	EntityID string
	Action   model.AuditAction
	DateFrom string
	DateTo   string
}

// AuditRepo is the GORM-backed AuditLogRepository.
type AuditRepo struct{ db *gorm.DB }

// NewAuditRepo builds an AuditRepo.
func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

// Create writes an audit entry (non-blocking by contract: caller wraps in error log).
// The LOG-### reference is derived from the row's own serial id inside the insert
// transaction, so it stays unique across restarts and concurrent writers.
func (r *AuditRepo) Create(ctx context.Context, entry *model.AuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Placeholder satisfies the NOT NULL unique column until we know the id.
		entry.LogRef = uuid.NewString()
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		ref := fmt.Sprintf("LOG-%03d", entry.ID)
		return tx.Model(&model.AuditLog{}).Where("id = ?", entry.ID).
			Update("log_ref", ref).Error
	})
}

// List returns a page of audit entries plus the total count.
func (r *AuditRepo) List(ctx context.Context, f AuditFilter) ([]model.AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.AuditLog{})
	if f.EntityID != "" {
		q = q.Where("entity_ref = ?", f.EntityID)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.DateFrom != "" {
		q = q.Where("created_at >= ?", f.DateFrom)
	}
	if f.DateTo != "" {
		q = q.Where("created_at <= ?", f.DateTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.AuditLog
	err := q.Order("id DESC").
		Offset((f.Page - 1) * f.Limit).
		Limit(f.Limit).
		Find(&rows).Error
	return rows, total, err
}

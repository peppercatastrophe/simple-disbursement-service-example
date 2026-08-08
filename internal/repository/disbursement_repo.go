package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DisbursementRepo is the GORM-backed DisbursementRepository.
type DisbursementRepo struct{ db *gorm.DB }

// NewDisbursementRepo builds a DisbursementRepo.
func NewDisbursementRepo(db *gorm.DB) *DisbursementRepo { return &DisbursementRepo{db: db} }

// Create inserts a new disbursement.
func (r *DisbursementRepo) Create(ctx context.Context, d *model.Disbursement) error {
	return r.db.WithContext(ctx).Create(d).Error
}

// FindByID returns a non-deleted disbursement.
func (r *DisbursementRepo) FindByID(ctx context.Context, id int64) (*model.Disbursement, error) {
	var d model.Disbursement
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &d, err
}

// UpdateStatus applies a guarded status transition. `from` is the expected
// current status; the UPDATE only matches when the row still has that status,
// so a concurrent transition to the same target cannot both succeed.
func (r *DisbursementRepo) UpdateStatus(ctx context.Context, d *model.Disbursement, from model.DisbursementStatus) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.Disbursement{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", d.ID, from).
		Updates(map[string]any{
			"status":      d.Status,
			"approved_by": d.ApprovedBy,
			"updated_at":  time.Now(),
		})
	return res.RowsAffected, res.Error
}

// SoftDelete marks a PENDING disbursement as deleted.
func (r *DisbursementRepo) SoftDelete(ctx context.Context, id int64) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.Disbursement{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", id, model.StatusPending).
		Update("deleted_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns a page of disbursements matching the filter plus the total count.
func (r *DisbursementRepo) List(ctx context.Context, f ListFilter) ([]model.Disbursement, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Disbursement{}).
		Where("deleted_at IS NULL")

	if f.Search != "" {
		q = q.Where("recipient_name ILIKE ?", "%"+f.Search+"%")
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
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

	sortCol := "created_at"
	if f.SortBy != "" {
		sortCol = f.SortBy
	}
	order := "DESC"
	if f.SortOrder != "" {
		order = f.SortOrder
	}

	var rows []model.Disbursement
	err := q.Order(fmt.Sprintf("%s %s", sortCol, order)).
		Offset((f.Page - 1) * f.Limit).
		Limit(f.Limit).
		Find(&rows).Error
	return rows, total, err
}

// CreateIdempotent creates a disbursement keyed by idempotencyKey, or returns
// the disbursement already created for that key. `replayed` is true when the
// key was already used, in which case no new row is inserted.
//
// Concurrency: the key is claimed with an INSERT ... ON CONFLICT DO NOTHING
// inside the same transaction that inserts the disbursement. Only the winning
// transaction inserts the key row and proceeds; a concurrent request with the
// same key falls to the replay path and blocks on a row lock until the winner
// commits, then returns the committed disbursement.
func (r *DisbursementRepo) CreateIdempotent(ctx context.Context, idempotencyKey string, d *model.Disbursement) (*model.Disbursement, bool, error) {
	var result *model.Disbursement
	replayed := false

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Table("idempotency_keys").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(map[string]any{
				"idempotency_key": idempotencyKey,
				"status":          "in_progress",
				"created_at":      time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}

		if res.RowsAffected == 1 {
			// Winner: insert the disbursement, then link the key.
			if err := tx.Create(d).Error; err != nil {
				return err
			}
			if err := tx.Table("idempotency_keys").
				Where("idempotency_key = ?", idempotencyKey).
				Updates(map[string]any{"disbursement_id": d.ID, "status": "committed"}).Error; err != nil {
				return err
			}
			result = d
			return nil
		}

		// Loser: key already claimed. Lock the key row (blocks until the winner
		// commits), then return the stored disbursement.
		var k struct {
			DisbursementID int64
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table("idempotency_keys").
			Select("disbursement_id").
			Where("idempotency_key = ?", idempotencyKey).
			First(&k).Error; err != nil {
			return err
		}
		var stored model.Disbursement
		if err := tx.First(&stored, k.DisbursementID).Error; err != nil {
			return err
		}
		result = &stored
		replayed = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}
	return result, replayed, nil
}

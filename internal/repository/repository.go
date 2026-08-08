package repository

import (
	"context"

	"github.com/asa/simple-disbursement-service-example/internal/model"
)

// UserRepository defines data access for users.
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

// DisbursementRepository defines data access for disbursements.
type DisbursementRepository interface {
	Create(ctx context.Context, d *model.Disbursement) error
	FindByID(ctx context.Context, id int64) (*model.Disbursement, error)
	// UpdateStatus performs the concurrency-safe status transition. It returns
	// the affected row count so the service can detect a lost race (0 rows).
	UpdateStatus(ctx context.Context, d *model.Disbursement, from model.DisbursementStatus) (int64, error)
	SoftDelete(ctx context.Context, id int64) error
	List(ctx context.Context, filter ListFilter) ([]model.Disbursement, int64, error)
	// CreateIdempotent creates a disbursement under an idempotency key, or
	// returns the already-created one (replayed=true) on a duplicate key.
	CreateIdempotent(ctx context.Context, key string, d *model.Disbursement) (*model.Disbursement, bool, error)
}

// ListFilter carries pagination and filtering parameters.
type ListFilter struct {
	Page      int
	Limit     int
	Search    string
	Status    model.DisbursementStatus
	DateFrom  string
	DateTo    string
	SortBy    string
	SortOrder string
}

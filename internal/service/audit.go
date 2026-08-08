package service

import (
	"context"
	"fmt"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
)

// AuditService records state changes. Writes are best-effort and MUST NOT
// fail the caller: if an audit write fails it is logged and the primary
// operation still succeeds (see ARCHITECTURE.md).
type AuditService struct {
	repo repository.AuditLogRepository
	// entityRef maps a disbursement id to a DSB-### reference.
	entityRef func(id int64) string
}

// NewAuditService builds an AuditService.
func NewAuditService(repo repository.AuditLogRepository) *AuditService {
	return &AuditService{
		repo: repo,
		entityRef: func(id int64) string {
			return fmt.Sprintf("DSB-%03d", id)
		},
	}
}

// RecordStatusChange logs a status transition; errors are swallowed by design.
func (s *AuditService) RecordStatusChange(ctx context.Context, actor string, id int64, before, after model.DisbursementStatus) {
	entry := &model.AuditLog{
		EntityRef: s.entityRef(id),
		Action:    model.ActionStatusChange,
		Actor:     actor,
		Before:    model.JSONMap{"status": before},
		After:     model.JSONMap{"status": after},
	}
	// Best-effort: ignore error, the operation must not fail.
	_ = s.repo.Create(ctx, entry)
}

// RecordCreate logs a new disbursement.
func (s *AuditService) RecordCreate(ctx context.Context, actor string, id int64, status model.DisbursementStatus) {
	entry := &model.AuditLog{
		EntityRef: s.entityRef(id),
		Action:    model.ActionCreated,
		Actor:     actor,
		After:     model.JSONMap{"status": status},
	}
	_ = s.repo.Create(ctx, entry)
}

// RecordDelete logs a soft delete.
func (s *AuditService) RecordDelete(ctx context.Context, actor string, id int64, before model.DisbursementStatus) {
	entry := &model.AuditLog{
		EntityRef: s.entityRef(id),
		Action:    model.ActionDeleted,
		Actor:     actor,
		Before:    model.JSONMap{"status": before},
	}
	_ = s.repo.Create(ctx, entry)
}

// List returns a page of audit entries plus the total count.
func (s *AuditService) List(ctx context.Context, filter repository.AuditFilter) ([]model.AuditLog, int64, error) {
	return s.repo.List(ctx, filter)
}

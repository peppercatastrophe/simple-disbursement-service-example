package handler

import (
	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
)

// AuditHandler exposes the audit log endpoint (superadmin).
type AuditHandler struct {
	svc *service.AuditService
}

// NewAuditHandler builds an AuditHandler.
func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List handles GET /audit-logs with pagination/filtering.
func (h *AuditHandler) List(c *gin.Context) {
	filter := repository.AuditFilter{
		Page:     parseDefaultInt(c.Query("page"), 1),
		Limit:    parseDefaultInt(c.Query("limit"), 20),
		EntityID: c.Query("entity_id"),
		Action:   model.AuditAction(c.Query("action")),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		RespondError(c, service.ErrBadLimit)
		return
	}
	rows, total, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		RespondError(c, err)
		return
	}
	meta := gin.H{
		"page":        filter.Page,
		"limit":       filter.Limit,
		"total":       total,
		"total_pages": pages(int(total), filter.Limit),
	}
	RespondOK(c, rows, meta)
}

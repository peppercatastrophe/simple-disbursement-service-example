package handler

import (
	"net/http"
	"strconv"

	"github.com/asa/simple-disbursement-service-example/internal/middleware"
	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
)

// DisbursementHandler exposes the disbursement CRUD endpoints.
type DisbursementHandler struct {
	svc *service.DisbursementService
}

// NewDisbursementHandler builds a DisbursementHandler.
func NewDisbursementHandler(svc *service.DisbursementService) *DisbursementHandler {
	return &DisbursementHandler{svc: svc}
}

type createDisbursementRequest struct {
	RecipientName string `json:"recipient_name" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required"`
	BankCode      string `json:"bank_code" binding:"required"`
	Amount        int64  `json:"amount" binding:"required"`
	Note          string `json:"note"`
}

type statusRequest struct {
	Status model.DisbursementStatus `json:"status" binding:"required"`
	Note   string                   `json:"note"`
}

// Create handles POST /disbursements with idempotency support.
func (h *DisbursementHandler) Create(c *gin.Context) {
	var req createDisbursementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, err)
		return
	}
	actor := middleware.CurrentUser(c)
	in := service.NewDisbursement{
		RecipientName: req.RecipientName,
		AccountNumber: req.AccountNumber,
		BankCode:      req.BankCode,
		Amount:        req.Amount,
		Note:          req.Note,
	}
	idemKey := c.GetHeader("Idempotency-Key")
	d, replayed, err := h.svc.Create(c.Request.Context(), actor, in, idemKey)
	if err != nil {
		RespondError(c, err)
		return
	}
	if replayed {
		c.Header("X-Idempotent-Replayed", "true")
	}
	RespondOK(c, d, nil)
}

// Get handles GET /disbursements/:id.
func (h *DisbursementHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, err)
		return
	}
	d, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		RespondError(c, err)
		return
	}
	RespondOK(c, d, nil)
}

// List handles GET /disbursements with pagination/filtering.
func (h *DisbursementHandler) List(c *gin.Context) {
	filter := repository.ListFilter{
		Page:      parseDefaultInt(c.Query("page"), 1),
		Limit:     parseDefaultInt(c.Query("limit"), 20),
		Search:    c.Query("search"),
		Status:    model.DisbursementStatus(c.Query("status")),
		DateFrom:  c.Query("date_from"),
		DateTo:    c.Query("date_to"),
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
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

// UpdateStatus handles PATCH /disbursements/:id/status.
func (h *DisbursementHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, err)
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, err)
		return
	}
	actor := middleware.CurrentUser(c)
	d, err := h.svc.UpdateStatus(c.Request.Context(), actor, id, req.Status, req.Note)
	if err != nil {
		RespondError(c, err)
		return
	}
	RespondOK(c, d, nil)
}

// Delete handles DELETE /disbursements/:id (superadmin soft delete).
func (h *DisbursementHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, err)
		return
	}
	actor := middleware.CurrentUser(c)
	if err := h.svc.Delete(c.Request.Context(), actor, id); err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
}

func parseDefaultInt(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func pages(total, limit int) int {
	if limit == 0 {
		return 0
	}
	return (total + limit - 1) / limit
}

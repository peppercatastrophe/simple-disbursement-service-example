package model

import "time"

// DisbursementStatus enumerates valid lifecycle states.
type DisbursementStatus string

const (
	StatusPending  DisbursementStatus = "PENDING"
	StatusApproved DisbursementStatus = "APPROVED"
	StatusRejected DisbursementStatus = "REJECTED"
)

// Disbursement is a single payout record.
type Disbursement struct {
	ID            int64              `gorm:"primaryKey" json:"id"`
	RecipientName string             `gorm:"not null" json:"recipient_name"`
	AccountNumber string             `gorm:"not null" json:"account_number"`
	BankCode      string             `gorm:"not null;index" json:"bank_code"`
	Amount        int64              `gorm:"not null;check:amount >= 10000" json:"amount"`
	AdminFee      int64              `gorm:"not null" json:"admin_fee"`
	Note          string             `json:"note,omitempty"`
	Status        DisbursementStatus `gorm:"not null;index" json:"status"`
	CreatedBy     int64              `json:"created_by"`
	ApprovedBy    *int64             `json:"approved_by,omitempty"`
	DeletedAt     *time.Time         `gorm:"index" json:"-"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// TableName overrides GORM's pluralization.
func (Disbursement) TableName() string { return "disbursements" }

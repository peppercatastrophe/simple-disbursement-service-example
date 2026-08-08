package service

import (
	"context"
	"errors"
	"time"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
)

// Sentinel errors mapped to HTTP status codes by the handler layer.
var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidToken        = errors.New("invalid or revoked token")
	ErrNotFound            = errors.New("not found")
	ErrAlreadyTransitioned = errors.New("disbursement already transitioned")
	ErrForbidden           = errors.New("forbidden")
	ErrBadLimit            = errors.New("limit must be between 1 and 100")
)

// UserService handles authentication and token lifecycle.
type UserService struct {
	users  repository.UserRepository
	tokens *TokenManager
}

// NewUserService builds a UserService.
func NewUserService(users repository.UserRepository, tm *TokenManager) *UserService {
	return &UserService{users: users, tokens: tm}
}

// Login authenticates a username/password and returns a token pair.
func (s *UserService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !verifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return s.tokens.IssuePair(ctx, u)
}

// Refresh swaps a valid refresh token for a new access token.
func (s *UserService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.tokens.Refresh(ctx, refreshToken)
}

// Logout revokes a refresh token.
func (s *UserService) Logout(ctx context.Context, refreshToken string) error {
	return s.tokens.Revoke(ctx, refreshToken)
}

// AdminFee computes the service fee from a gross amount.
// 2500 below 5,000,000; 5000 at or above.
func AdminFee(amount int64) int64 {
	if amount >= 5_000_000 {
		return 5000
	}
	return 2500
}

// DisbursementService implements disbursement business rules.
type DisbursementService struct {
	repo  repository.DisbursementRepository
	audit *AuditService
}

// NewDisbursementService builds a DisbursementService.
func NewDisbursementService(repo repository.DisbursementRepository, audit *AuditService) *DisbursementService {
	return &DisbursementService{repo: repo, audit: audit}
}

// Create validates and creates a disbursement. If idemKey is non-empty, the
// create is idempotent: a duplicate key returns the original disbursement with
// replayed=true instead of creating a new row.
func (s *DisbursementService) Create(ctx context.Context, actor *model.User, in NewDisbursement, idemKey string) (*model.Disbursement, bool, error) {
	if err := in.Validate(); err != nil {
		return nil, false, err
	}
	d := &model.Disbursement{
		RecipientName: in.RecipientName,
		AccountNumber: in.AccountNumber,
		BankCode:      in.BankCode,
		Amount:        in.Amount,
		AdminFee:      AdminFee(in.Amount),
		Note:          in.Note,
		Status:        model.StatusPending,
		CreatedBy:     actor.ID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if idemKey != "" {
		res, replayed, err := s.repo.CreateIdempotent(ctx, idemKey, d)
		if err != nil {
			return nil, false, err
		}
		if !replayed {
			s.audit.RecordCreate(ctx, actor.Username, res.ID, res.Status)
		}
		return res, replayed, nil
	}

	if err := s.repo.Create(ctx, d); err != nil {
		return nil, false, err
	}
	s.audit.RecordCreate(ctx, actor.Username, d.ID, d.Status)
	return d, false, nil
}

// UpdateStatus transitions a disbursement's status in a concurrency-safe way.
func (s *DisbursementService) UpdateStatus(ctx context.Context, actor *model.User, id int64, target model.DisbursementStatus, note string) (*model.Disbursement, error) {
	if target != model.StatusApproved && target != model.StatusRejected {
		return nil, errors.New("status must be APPROVED or REJECTED")
	}
	if actor.Role != model.RoleAdmin && actor.Role != model.RoleSuperadmin {
		return nil, ErrForbidden
	}
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if d.Status != model.StatusPending {
		return nil, ErrAlreadyTransitioned
	}
	var approvedBy *int64
	if target == model.StatusApproved {
		approvedBy = &actor.ID
	}
	before := d.Status
	d.Status = target
	d.ApprovedBy = approvedBy
	n, err := s.repo.UpdateStatus(ctx, d, before)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrAlreadyTransitioned
	}
	s.audit.RecordStatusChange(ctx, actor.Username, id, before, target)
	return d, nil
}

// Get returns a single disbursement.
func (s *DisbursementService) Get(ctx context.Context, id int64) (*model.Disbursement, error) {
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return d, nil
}

// List returns a page of disbursements plus the total count.
func (s *DisbursementService) List(ctx context.Context, filter repository.ListFilter) ([]model.Disbursement, int64, error) {
	return s.repo.List(ctx, filter)
}

// Delete soft-deletes a PENDING disbursement (superadmin only).
func (s *DisbursementService) Delete(ctx context.Context, actor *model.User, id int64) error {
	if actor.Role != model.RoleSuperadmin {
		return ErrForbidden
	}
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if d.Status != model.StatusPending {
		return ErrAlreadyTransitioned
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAlreadyTransitioned
		}
		return err
	}
	s.audit.RecordDelete(ctx, actor.Username, id, d.Status)
	return nil
}

// NewDisbursement is the validated request payload for creating a disbursement.
type NewDisbursement struct {
	RecipientName string
	AccountNumber string
	BankCode      string
	Amount        int64
	Note          string
}

// Validate enforces required fields and the minimum amount.
func (n NewDisbursement) Validate() error {
	switch {
	case n.RecipientName == "":
		return errors.New("recipient_name is required")
	case n.AccountNumber == "":
		return errors.New("account_number is required")
	case n.BankCode == "":
		return errors.New("bank_code is required")
	case n.Amount < 10000:
		return errors.New("amount must be a positive integer of at least 10000")
	}
	return nil
}

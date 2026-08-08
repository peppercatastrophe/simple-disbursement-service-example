package repository

import (
	"context"
	"errors"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a row does not exist (or is soft-deleted).
var ErrNotFound = errors.New("record not found")

// UserRepo is the GORM-backed UserRepository.
type UserRepo struct{ db *gorm.DB }

// NewUserRepo builds a UserRepo.
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// FindByUsername returns the user with the given username.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

// FindByID returns the user with the given id.
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

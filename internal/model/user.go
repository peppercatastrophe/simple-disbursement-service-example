package model

import "time"

// Role enumerates the supported user roles.
type Role string

const (
	RoleSuperadmin Role = "superadmin"
	RoleAdmin      Role = "admin"
	RoleOperator   Role = "operator"
)

// User is a staff account that can authenticate and act on disbursements.
type User struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         Role      `gorm:"not null" json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName overrides GORM's pluralization.
func (User) TableName() string { return "users" }

package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// AuditAction enumerates audited operations.
type AuditAction string

const (
	ActionCreated      AuditAction = "created"
	ActionStatusChange AuditAction = "status_changed"
	ActionDeleted      AuditAction = "deleted"
)

// JSONMap is a map that (de)serializes as a JSON object column.
type JSONMap map[string]any

// Value implements driver.Valuer for Postgres jsonb columns.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner for Postgres jsonb columns.
func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = JSONMap{}
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return fmt.Errorf("unsupported scan type %T", src)
	}
}

// AuditLog records a state-changing operation on a disbursement.
type AuditLog struct {
	ID        int64       `gorm:"primaryKey" json:"-"`
	LogRef    string      `gorm:"uniqueIndex;not null" json:"id"`  // LOG-001
	EntityRef string      `gorm:"index;not null" json:"entity_id"` // DSB-001
	Action    AuditAction `gorm:"not null;index" json:"action"`
	Actor     string      `gorm:"not null" json:"actor"`
	Before    JSONMap     `gorm:"type:jsonb" json:"before"`
	After     JSONMap     `gorm:"type:jsonb" json:"after"`
	CreatedAt time.Time   `json:"created_at"`
}

// TableName overrides GORM's pluralization.
func (AuditLog) TableName() string { return "audit_logs" }

package models

import "time"

type AdminUser struct {
	ID           string `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Username     string `gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"size:32;not null"`
	Status       string `gorm:"size:32;not null"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (AdminUser) TableName() string {
	return "admin_users"
}

type AdminSession struct {
	ID          string `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	AdminUserID string `gorm:"type:uuid;not null"`
	TokenHash   string `gorm:"uniqueIndex;not null"`
	IP          string `gorm:"type:inet"`
	UserAgent   string
	ExpiresAt   time.Time `gorm:"not null"`
	CreatedAt   time.Time
}

func (AdminSession) TableName() string {
	return "admin_sessions"
}

type AuditLog struct {
	ID          string  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	AdminUserID *string `gorm:"type:uuid"`
	Action      string  `gorm:"size:128;not null"`
	TargetType  string  `gorm:"size:64"`
	TargetID    string  `gorm:"size:128"`
	IP          string  `gorm:"type:inet"`
	UserAgent   string
	DetailJSON  string `gorm:"column:detail_json;type:jsonb;not null"`
	CreatedAt   time.Time
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

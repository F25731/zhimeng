package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/models"
	"github.com/F25731/zhimeng/backend/internal/security"
	"gorm.io/gorm"
)

const (
	SessionCookieName = "control_admin_session"
	CSRFHeaderName    = "X-CSRF-Token"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginLocked        = errors.New("too many login attempts")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
)

type Service struct {
	cfg config.Config
	db  *gorm.DB
}

type SessionResult struct {
	User      models.AdminUser
	Token     string
	ExpiresAt time.Time
}

func NewService(cfg config.Config, db *gorm.DB) *Service {
	return &Service{cfg: cfg, db: db}
}

func (s *Service) Login(username string, password string, ip string, userAgent string) (SessionResult, error) {
	username = strings.TrimSpace(username)
	var recentFailures int64
	if err := s.db.Raw(`SELECT count(*) FROM admin_login_attempts WHERE username=? AND succeeded=false AND created_at>now()-interval '15 minutes'`, username).Scan(&recentFailures).Error; err != nil {
		return SessionResult{}, err
	}
	if recentFailures >= 5 {
		return SessionResult{}, ErrLoginLocked
	}
	var user models.AdminUser
	if err := s.db.Where("username = ? AND status = ?", username, "active").First(&user).Error; err != nil {
		s.recordLoginAttempt(username, ip, false)
		return SessionResult{}, ErrInvalidCredentials
	}
	if !security.VerifyPassword(user.PasswordHash, password) {
		s.recordLoginAttempt(username, ip, false)
		return SessionResult{}, ErrInvalidCredentials
	}

	token, err := security.RandomToken(32)
	if err != nil {
		return SessionResult{}, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)
	session := models.AdminSession{
		AdminUserID: user.ID,
		TokenHash:   security.SHA256Hex(token),
		IP:          ip,
		UserAgent:   userAgent,
		ExpiresAt:   expiresAt,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM admin_login_attempts WHERE username=? AND succeeded=false`, username).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO admin_login_attempts(username,ip,succeeded) VALUES (?,NULLIF(?,'')::inet,true)`, username, ip).Error; err != nil {
			return err
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.AdminUser{}).Where("id = ?", user.ID).Update("last_login_at", now).Error; err != nil {
			return err
		}
		return createAuditLog(tx, &user.ID, "admin.login", "admin_user", user.ID, ip, userAgent, `{"result":"success"}`)
	})
	if err != nil {
		return SessionResult{}, err
	}

	user.LastLoginAt = &now
	return SessionResult{User: user, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) recordLoginAttempt(username, ip string, succeeded bool) {
	_ = s.db.Exec(`INSERT INTO admin_login_attempts(username,ip,succeeded) VALUES (?,NULLIF(?,'')::inet,?)`, username, ip, succeeded).Error
}

func (s *Service) Authenticate(token string) (models.AdminUser, models.AdminSession, error) {
	if strings.TrimSpace(token) == "" {
		return models.AdminUser{}, models.AdminSession{}, ErrUnauthorized
	}

	var session models.AdminSession
	if err := s.db.Where("token_hash = ? AND expires_at > ?", security.SHA256Hex(token), time.Now().UTC()).First(&session).Error; err != nil {
		return models.AdminUser{}, models.AdminSession{}, ErrUnauthorized
	}

	var user models.AdminUser
	if err := s.db.Where("id = ? AND status = ?", session.AdminUserID, "active").First(&user).Error; err != nil {
		return models.AdminUser{}, models.AdminSession{}, ErrUnauthorized
	}

	return user, session, nil
}

func (s *Service) Logout(token string, userID string, ip string, userAgent string) error {
	hash := security.SHA256Hex(token)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_hash = ?", hash).Delete(&models.AdminSession{}).Error; err != nil {
			return err
		}
		return createAuditLog(tx, &userID, "admin.logout", "admin_user", userID, ip, userAgent, `{}`)
	})
}

func (s *Service) CSRFToken(sessionToken string) string {
	secret := s.cfg.CSRFSecret
	if secret == "" {
		secret = s.cfg.SessionSecret
	}
	return security.HMACSHA256Base64(sessionToken, secret)
}

func (s *Service) VerifyCSRF(sessionToken string, csrfToken string) bool {
	return security.ConstantEqual(s.CSRFToken(sessionToken), csrfToken)
}

func createAuditLog(tx *gorm.DB, adminUserID *string, action string, targetType string, targetID string, ip string, userAgent string, detail string) error {
	if detail == "" {
		detail = `{}`
	}
	return tx.Exec(
		`INSERT INTO audit_logs (admin_user_id, action, target_type, target_id, ip, user_agent, detail_json)
		 VALUES (?, ?, ?, ?, NULLIF(?, '')::inet, ?, ?::jsonb)`,
		adminUserID,
		action,
		targetType,
		targetID,
		ip,
		userAgent,
		detail,
	).Error
}

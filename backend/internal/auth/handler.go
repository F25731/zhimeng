package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/F25731/zhimeng/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	cfg     config.Config
	service *Service
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewHandler(cfg config.Config, service *Service) *Handler {
	return &Handler{cfg: cfg, service: service}
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, 40000, "invalid request")
		return
	}

	result, err := h.service.Login(req.Username, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if errors.Is(err, ErrLoginLocked) {
			c.Header("Retry-After", "900")
			httpx.Error(c, http.StatusTooManyRequests, 42901, "登录失败次数过多，请 15 分钟后重试")
			return
		}
		if errors.Is(err, ErrInvalidCredentials) {
			httpx.Error(c, http.StatusUnauthorized, 40101, "invalid username or password")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, 50000, "login failed")
		return
	}

	h.setSessionCookie(c, result.Token, int((30 * 24 * time.Hour).Seconds()))
	httpx.OK(c, gin.H{"admin": publicAdmin(result.User), "csrfToken": h.service.CSRFToken(result.Token)})
}

func (h *Handler) Me(c *gin.Context) {
	user := c.MustGet("admin_user").(models.AdminUser)
	sessionToken := c.MustGet("admin_session_token").(string)
	httpx.OK(c, gin.H{"admin": publicAdmin(user), "csrfToken": h.service.CSRFToken(sessionToken)})
}

func (h *Handler) Logout(c *gin.Context) {
	user := c.MustGet("admin_user").(models.AdminUser)
	token := c.MustGet("admin_session_token").(string)
	if err := h.service.Logout(token, user.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		httpx.Error(c, http.StatusInternalServerError, 50000, "logout failed")
		return
	}
	h.clearSessionCookie(c)
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) setSessionCookie(c *gin.Context, token string, maxAge int) {
	secure := h.cfg.AppEnv != "development"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookieName, token, maxAge, "/", "", secure, true)
}

func (h *Handler) clearSessionCookie(c *gin.Context) {
	secure := h.cfg.AppEnv != "development"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookieName, "", -1, "/", "", secure, true)
}

func publicAdmin(user models.AdminUser) gin.H {
	return gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"role":        user.Role,
		"status":      user.Status,
		"lastLoginAt": user.LastLoginAt,
	}
}

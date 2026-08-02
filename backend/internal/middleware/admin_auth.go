package middleware

import (
	"net/http"

	"github.com/F25731/zhimeng/backend/internal/auth"
	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

func AdminAuth(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(auth.SessionCookieName)
		if err != nil || token == "" {
			httpx.Error(c, http.StatusUnauthorized, 40100, "login required")
			c.Abort()
			return
		}

		user, session, err := service.Authenticate(token)
		if err != nil {
			httpx.Error(c, http.StatusUnauthorized, 40100, "login required")
			c.Abort()
			return
		}

		c.Set("admin_user", user)
		c.Set("admin_session", session)
		c.Set("admin_session_token", token)
		c.Next()
	}
}

func AdminCSRF(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		token, exists := c.Get("admin_session_token")
		if !exists {
			httpx.Error(c, http.StatusUnauthorized, 40100, "login required")
			c.Abort()
			return
		}

		csrfToken := c.GetHeader(auth.CSRFHeaderName)
		if !service.VerifyCSRF(token.(string), csrfToken) {
			httpx.Error(c, http.StatusForbidden, 40300, "csrf validation failed")
			c.Abort()
			return
		}
		c.Next()
	}
}

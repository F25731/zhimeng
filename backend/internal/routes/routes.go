package routes

import (
	"context"
	"log/slog"
	"time"

	"github.com/F25731/zhimeng/backend/internal/auth"
	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/control"
	"github.com/F25731/zhimeng/backend/internal/database"
	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/F25731/zhimeng/backend/internal/middleware"
	"github.com/F25731/zhimeng/backend/pkg/version"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewAPI(cfg config.Config, db *gorm.DB, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID())
	router.Static("/uploads", "./uploads")
	authService := auth.NewService(cfg, db)
	authHandler := auth.NewHandler(cfg, authService)
	controlService := control.NewService(cfg, db)
	controlHandler := control.NewHandler(controlService)

	router.GET("/api/health/live", func(c *gin.Context) {
		httpx.OK(c, gin.H{"service": "control-api", "version": version.Version})
	})

	router.GET("/api/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := database.Ping(ctx, db); err != nil {
			logger.Warn("database readiness failed", "error", err)
			httpx.Error(c, 503, 50301, "database not ready")
			return
		}
		httpx.OK(c, gin.H{"database": "ok"})
	})

	public := router.Group("/api/public")
	public.Use(middleware.RateLimitIP(60, time.Minute))
	{
		public.POST("/provision/codes/verify", controlHandler.VerifyCode)
		public.POST("/provision/domain/check", controlHandler.CheckDomain)
		public.POST("/provision/logo/upload", controlHandler.UploadLogo)
		public.POST("/provision/logo/import", controlHandler.ImportLogo)
		public.POST("/provision/jobs", controlHandler.CreateJob)
		public.GET("/provision/jobs/:id", controlHandler.GetPublicJob)
		public.GET("/provision/jobs/:id/events", controlHandler.PublicEvents)
		public.POST("/provision/jobs/:id/retry", controlHandler.RetryPublicJob)
	}

	admin := router.Group("/api/admin")
	{
		admin.POST("/auth/login", authHandler.Login)

		protected := admin.Group("")
		protected.Use(middleware.AdminAuth(authService), middleware.AdminCSRF(authService))
		protected.GET("/auth/me", authHandler.Me)
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/dashboard", controlHandler.Dashboard)
		protected.GET("/codes", controlHandler.ListCodes)
		protected.POST("/codes", controlHandler.CreateCodes)
		protected.POST("/codes/batch", controlHandler.CreateCodes)
		protected.POST("/codes/:id/revoke", controlHandler.RevokeCode)
		protected.DELETE("/codes/:id", controlHandler.DeleteCode)
		protected.GET("/codes/export", controlHandler.ExportCodes)
		protected.GET("/sites", controlHandler.ListSites)
		protected.GET("/sites/:id", controlHandler.SiteDetail)
		protected.GET("/sites/:id/metrics", controlHandler.SiteMetrics)
		protected.GET("/sites/:id/channels", controlHandler.SiteChannels)
		protected.POST("/sites/:id/:action", controlHandler.SiteAction)
		protected.GET("/jobs", controlHandler.ListJobs)
		protected.GET("/jobs/:id", controlHandler.GetJob)
		protected.GET("/jobs/:id/events", controlHandler.JobEvents)
		protected.POST("/jobs/:id/retry", controlHandler.RetryJob)
		protected.GET("/versions", controlHandler.ListVersions)
		protected.POST("/versions", controlHandler.CreateVersion)
		protected.POST("/versions/:id/publish", controlHandler.PublishVersion)
		protected.GET("/nodes", controlHandler.ListNodes)
		protected.GET("/audit", controlHandler.ListAudit)
	}

	internal := router.Group("/api/internal/sites")
	{
		internal.POST("/heartbeat", controlHandler.Heartbeat)
		internal.POST("/metrics", controlHandler.Metrics)
		internal.POST("/channels", controlHandler.Channels)
	}

	_ = cfg
	return router
}

func notImplemented(message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpx.Error(c, 501, 50100, message)
	}
}

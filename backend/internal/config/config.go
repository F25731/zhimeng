package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv                    string
	AppPort                   int
	PublicBaseURL             string
	RootDomain                string
	AdminPath                 string
	DatabaseURL               string
	SessionSecret             string
	CSRFSecret                string
	CardHashSecret            string
	MasterEncryptionKey       string
	PostgresAdminURL          string
	DockerNetwork             string
	DatabaseNetwork           string
	SiteBaseDir               string
	SiteImageDefault          string
	ReporterImage             string
	SiteTemplateDir           string
	SiteRouterURL             string
	VerifyPublicHTTPS         bool
	RouteProbeTimeoutSeconds  int
	WorkerReadyTimeoutSeconds int
	AgentID                   string
	AgentPollIntervalSeconds  int
	AgentLeaseSeconds         int
	SSEHeartbeatSeconds       int
	SiteWarningSeconds        int
	SiteOfflineSeconds        int
	ObjectStorageEndpoint     string
	ObjectStorageRegion       string
	ObjectStorageBucket       string
	ObjectStorageAccessKey    string
	ObjectStorageSecretKey    string
	ObjectStoragePublicURL    string
}

func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("..")
	v.AutomaticEnv()

	setDefaults(v)
	_ = v.ReadInConfig()

	cfg := Config{
		AppEnv:                    v.GetString("APP_ENV"),
		AppPort:                   v.GetInt("APP_PORT"),
		PublicBaseURL:             v.GetString("PUBLIC_BASE_URL"),
		RootDomain:                v.GetString("ROOT_DOMAIN"),
		AdminPath:                 v.GetString("ADMIN_PATH"),
		DatabaseURL:               v.GetString("DATABASE_URL"),
		SessionSecret:             v.GetString("SESSION_SECRET"),
		CSRFSecret:                v.GetString("CSRF_SECRET"),
		CardHashSecret:            v.GetString("CARD_HASH_SECRET"),
		MasterEncryptionKey:       v.GetString("MASTER_ENCRYPTION_KEY"),
		PostgresAdminURL:          v.GetString("POSTGRES_ADMIN_URL"),
		DockerNetwork:             v.GetString("DOCKER_NETWORK"),
		DatabaseNetwork:           v.GetString("DATABASE_NETWORK"),
		SiteBaseDir:               v.GetString("SITE_BASE_DIR"),
		SiteImageDefault:          v.GetString("SITE_IMAGE_DEFAULT"),
		ReporterImage:             v.GetString("REPORTER_IMAGE"),
		SiteTemplateDir:           v.GetString("SITE_TEMPLATE_DIR"),
		SiteRouterURL:             v.GetString("SITE_ROUTER_URL"),
		VerifyPublicHTTPS:         v.GetBool("VERIFY_PUBLIC_HTTPS"),
		RouteProbeTimeoutSeconds:  v.GetInt("ROUTE_PROBE_TIMEOUT_SECONDS"),
		WorkerReadyTimeoutSeconds: v.GetInt("WORKER_READY_TIMEOUT_SECONDS"),
		AgentID:                   v.GetString("AGENT_ID"),
		AgentPollIntervalSeconds:  v.GetInt("AGENT_POLL_INTERVAL_SECONDS"),
		AgentLeaseSeconds:         v.GetInt("AGENT_LEASE_SECONDS"),
		SSEHeartbeatSeconds:       v.GetInt("SSE_HEARTBEAT_SECONDS"),
		SiteWarningSeconds:        v.GetInt("SITE_WARNING_SECONDS"),
		SiteOfflineSeconds:        v.GetInt("SITE_OFFLINE_SECONDS"),
		ObjectStorageEndpoint:     v.GetString("OBJECT_STORAGE_ENDPOINT"),
		ObjectStorageRegion:       v.GetString("OBJECT_STORAGE_REGION"),
		ObjectStorageBucket:       v.GetString("OBJECT_STORAGE_BUCKET"),
		ObjectStorageAccessKey:    v.GetString("OBJECT_STORAGE_ACCESS_KEY"),
		ObjectStorageSecretKey:    v.GetString("OBJECT_STORAGE_SECRET_KEY"),
		ObjectStoragePublicURL:    v.GetString("OBJECT_STORAGE_PUBLIC_URL"),
	}

	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	cfg.RootDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cfg.RootDomain), "."))
	cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	cfg.SiteRouterURL = strings.TrimRight(strings.TrimSpace(cfg.SiteRouterURL), "/")
	if cfg.AppEnv == "production" {
		required := map[string]string{
			"PUBLIC_BASE_URL": cfg.PublicBaseURL, "ROOT_DOMAIN": cfg.RootDomain,
			"POSTGRES_ADMIN_URL": cfg.PostgresAdminURL, "SITE_IMAGE_DEFAULT": cfg.SiteImageDefault,
			"REPORTER_IMAGE": cfg.ReporterImage, "SITE_TEMPLATE_DIR": cfg.SiteTemplateDir,
			"SITE_ROUTER_URL": cfg.SiteRouterURL,
		}
		for name, value := range required {
			if strings.TrimSpace(value) == "" {
				return Config{}, fmt.Errorf("%s is required in production", name)
			}
		}
		for name, value := range map[string]string{"PUBLIC_BASE_URL": cfg.PublicBaseURL, "SITE_ROUTER_URL": cfg.SiteRouterURL} {
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return Config{}, fmt.Errorf("%s must be an absolute HTTP URL", name)
			}
		}
		if cfg.RootDomain == "" || strings.ContainsAny(cfg.RootDomain, "/: ") || !strings.Contains(cfg.RootDomain, ".") {
			return Config{}, fmt.Errorf("ROOT_DOMAIN must be a hostname")
		}
		secrets := map[string]string{
			"SESSION_SECRET": cfg.SessionSecret, "CSRF_SECRET": cfg.CSRFSecret,
			"CARD_HASH_SECRET": cfg.CardHashSecret, "MASTER_ENCRYPTION_KEY": cfg.MasterEncryptionKey,
		}
		for name, value := range secrets {
			if len(strings.TrimSpace(value)) < 32 || strings.HasPrefix(value, "CHANGE_") || value == "replace_me" {
				return Config{}, fmt.Errorf("%s must be an independent secret of at least 32 characters", name)
			}
		}
	}
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_PORT", 8080)
	v.SetDefault("ADMIN_PATH", "/zhimeng")
	v.SetDefault("AGENT_ID", "node-001")
	v.SetDefault("AGENT_POLL_INTERVAL_SECONDS", 3)
	v.SetDefault("AGENT_LEASE_SECONDS", 30)
	v.SetDefault("SSE_HEARTBEAT_SECONDS", 15)
	v.SetDefault("SITE_WARNING_SECONDS", 60)
	v.SetDefault("SITE_OFFLINE_SECONDS", 120)
	v.SetDefault("DOCKER_NETWORK", "platform-proxy")
	v.SetDefault("DATABASE_NETWORK", "zhimeng-control-private")
	v.SetDefault("SITE_BASE_DIR", "/opt/platform/sites")
	v.SetDefault("SITE_TEMPLATE_DIR", "../deploy/templates")
	v.SetDefault("SITE_IMAGE_DEFAULT", "ghcr.io/example/site-app:latest")
	v.SetDefault("REPORTER_IMAGE", "zhimeng-control-backend:local")
	v.SetDefault("SITE_ROUTER_URL", "http://site-router")
	v.SetDefault("VERIFY_PUBLIC_HTTPS", true)
	v.SetDefault("ROUTE_PROBE_TIMEOUT_SECONDS", 90)
	v.SetDefault("WORKER_READY_TIMEOUT_SECONDS", 180)
}

func (c Config) HTTPAddr() string {
	return fmt.Sprintf(":%d", c.AppPort)
}

func (c Config) LogLevel() slog.Level {
	if c.AppEnv == "development" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

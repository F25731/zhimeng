package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/database"
	"github.com/F25731/zhimeng/backend/internal/models"
	"github.com/F25731/zhimeng/backend/internal/security"
)

func main() {
	username := flag.String("username", os.Getenv("ADMIN_USERNAME"), "admin username")
	password := flag.String("password", os.Getenv("ADMIN_PASSWORD"), "admin password")
	flag.Parse()

	if strings.TrimSpace(*username) == "" || strings.TrimSpace(*password) == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_USERNAME and ADMIN_PASSWORD are required")
		os.Exit(1)
	}
	if len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "admin password must contain at least 8 characters")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect database failed", "error", err)
		os.Exit(1)
	}

	hash, err := security.HashPassword(*password)
	if err != nil {
		slog.Error("hash password failed", "error", err)
		os.Exit(1)
	}

	user := models.AdminUser{
		Username:     strings.TrimSpace(*username),
		PasswordHash: hash,
		Role:         "super_admin",
		Status:       "active",
	}

	if err := db.Where("username = ?", user.Username).Assign(map[string]interface{}{
		"password_hash": hash,
		"role":          "super_admin",
		"status":        "active",
	}).FirstOrCreate(&user).Error; err != nil {
		slog.Error("create admin failed", "error", err)
		os.Exit(1)
	}
	if err := db.Where("admin_user_id = ?", user.ID).Delete(&models.AdminSession{}).Error; err != nil {
		slog.Error("invalidate previous sessions failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("admin ready: %s\n", user.Username)
}

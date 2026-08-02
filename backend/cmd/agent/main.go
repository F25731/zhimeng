package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/F25731/zhimeng/backend/internal/agent"
	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	slog.SetDefault(logger)

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	worker := agent.NewWorker(cfg, db, logger)
	if err := worker.Run(ctx); err != nil {
		logger.Error("control agent stopped", "error", err)
		os.Exit(1)
	}
}

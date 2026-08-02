package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/control"
	"gorm.io/gorm"
)

type Worker struct {
	cfg    config.Config
	db     *gorm.DB
	logger *slog.Logger
}

func NewWorker(cfg config.Config, db *gorm.DB, logger *slog.Logger) *Worker {
	return &Worker{cfg: cfg, db: db, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	interval := time.Duration(w.cfg.AgentPollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}

	w.logger.Info("control agent started", "agentId", w.cfg.AgentID, "pollInterval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	maintenanceTicker := time.NewTicker(time.Minute)
	defer maintenanceTicker.Stop()
	service := control.NewService(w.cfg, w.db)
	if err := service.UpdateAgentHeartbeat(); err != nil {
		w.logger.Warn("initial agent heartbeat failed", "error", err)
	}
	if err := service.RunMaintenance(); err != nil {
		w.logger.Warn("initial scheduled maintenance failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("control agent shutdown requested", "agentId", w.cfg.AgentID)
			return nil
		case <-maintenanceTicker.C:
			if err := service.UpdateAgentHeartbeat(); err != nil {
				w.logger.Warn("update agent heartbeat failed", "error", err)
			}
			if err := service.RunMaintenance(); err != nil {
				w.logger.Warn("run scheduled maintenance failed", "error", err)
			}
		case <-ticker.C:
			job, err := service.ClaimNextJob(w.cfg.AgentID, time.Duration(w.cfg.AgentLeaseSeconds)*time.Second)
			if control.IsNoJob(err) {
				continue
			}
			if err != nil {
				w.logger.Error("claim deployment job failed", "agentId", w.cfg.AgentID, "error", err)
				continue
			}
			w.logger.Info("executing deployment job", "jobId", job.ID, "siteId", job.SiteID, "type", job.JobType)
			if err := service.ExecuteClaimedJob(ctx, job); err != nil {
				w.logger.Error("deployment job failed", "jobId", job.ID, "error", err)
				if failErr := service.FailJob(job, "AGENT_EXECUTION_FAILED", "分站部署失败，可以重试", err.Error(), true); failErr != nil {
					w.logger.Error("mark deployment job failed", "jobId", job.ID, "error", failErr)
				}
			}
		}
	}
}

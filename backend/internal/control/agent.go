package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ClaimedJob struct {
	Job
	PayloadJSON string
}

func (s *Service) ClaimNextJob(agentID string, lease time.Duration) (ClaimedJob, error) {
	var job ClaimedJob
	err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Raw(`WITH candidate AS (
			SELECT id FROM deployment_jobs
			WHERE attempt<max_attempts AND (status='pending' OR (status='running' AND lease_until<now()))
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE deployment_jobs j
		SET status='running',worker_id=?,lease_until=now()+(?*interval '1 second'),
			started_at=COALESCE(started_at,now()),attempt=attempt+1,lease_version=lease_version+1,updated_at=now()
		FROM candidate WHERE j.id=candidate.id
		RETURNING j.id,j.site_id,j.job_type,j.status,j.current_step,j.progress,j.attempt,j.max_attempts,
			j.retryable,j.payload_json,j.worker_id,j.lease_version,j.created_at,j.updated_at`, agentID, int(lease.Seconds())).Scan(&job).Error
		if err != nil {
			return err
		}
		if job.ID == "" {
			return gorm.ErrRecordNotFound
		}
		return s.addEvent(tx, job.ID, "progress", job.CurrentStep, "running", job.Progress, "任务已开始执行", "")
	})
	return job, err
}

func (s *Service) ExecuteClaimedJob(ctx context.Context, job ClaimedJob) error {
	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-executionCtx.Done():
				return
			case <-ticker.C:
				result := s.db.Exec(`UPDATE deployment_jobs SET lease_until=now()+interval '30 seconds',updated_at=now()
					WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, job.ID, job.WorkerID, job.LeaseVersion)
				if result.Error != nil || result.RowsAffected != 1 {
					cancel()
					return
				}
			}
		}
	}()
	executor := newSiteExecutor(s.cfg, s.db)
	var err error
	if job.JobType == "provision" {
		err = s.runProvision(executionCtx, executor, job)
	} else {
		err = s.runSiteOperation(executionCtx, executor, job)
	}
	cancel()
	<-done
	return err
}

func (s *Service) runProvision(ctx context.Context, executor *siteExecutor, job ClaimedJob) (runErr error) {
	var site siteRuntime
	var credentials siteCredentials
	var image, dir string
	routeEnabled := false
	defer func() {
		if runErr == nil {
			return
		}
		var currentClaim int64
		_ = s.db.Raw(`SELECT COUNT(*) FROM deployment_jobs WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, job.ID, job.WorkerID, job.LeaseVersion).Scan(&currentClaim).Error
		if currentClaim == 1 {
			if routeEnabled && site.ID != "" && dir != "" && image != "" {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				if _, err := executor.renderSite(site, credentials, image, site.DesiredVersion, false); err == nil {
					_ = executor.compose(cleanupCtx, dir, "up", "-d", "--no-deps", "app")
				}
			}
			_ = s.db.Exec(`UPDATE sites SET route_status='failed',route_error=?,updated_at=now() WHERE id=?`, runErr.Error(), job.SiteID).Error
		}
	}()

	if err := s.db.Exec(`UPDATE sites SET status='provisioning',route_status='disabled',route_error=NULL,last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, job.SiteID).Error; err != nil {
		return err
	}
	if err := s.advance(job, "validating", 5, "正在校验开站参数"); err != nil {
		return err
	}
	var err error
	site, err = executor.loadSite(job.SiteID)
	if err != nil {
		return err
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return err
	}
	if payload["adminUsername"] == "" {
		return errors.New("bootstrap administrator is missing")
	}

	if err := s.advance(job, "allocating_node", 12, "正在分配部署节点"); err != nil {
		return err
	}
	if err := executor.ensureNode(site.ID); err != nil {
		return err
	}
	if err := s.advance(job, "generating_secrets", 20, "正在生成站点独立密钥"); err != nil {
		return err
	}
	credentials, err = executor.ensureCredentials(site.ID, true)
	if err != nil {
		return err
	}

	if err := s.advance(job, "creating_database", 30, "正在创建独立数据库"); err != nil {
		return err
	}
	if err := executor.ensureDatabase(ctx, site, credentials.DatabasePassword); err != nil {
		return err
	}
	image, err = executor.resolveImage(site.DesiredVersion)
	if err != nil {
		return err
	}
	if err := s.advance(job, "generating_config", 40, "正在生成独立配置"); err != nil {
		return err
	}
	dir, err = executor.renderSite(site, credentials, image, site.DesiredVersion, false)
	if err != nil {
		return err
	}

	if err := s.advance(job, "pulling_image", 48, "正在准备应用镜像"); err != nil {
		return err
	}
	if err := executor.prepareImages(ctx, dir); err != nil {
		return err
	}
	if err := s.advance(job, "starting_containers", 58, "正在内网启动独立应用和任务服务"); err != nil {
		return err
	}
	if err := executor.compose(ctx, dir, "up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	if err := executor.waitForHealth(ctx, site, false); err != nil {
		return err
	}

	if err := s.advance(job, "initializing_database", 70, "正在初始化数据并创建分站管理员"); err != nil {
		return err
	}
	if err := executor.bootstrap(ctx, site, payload["adminUsername"], credentials.AdminPassword, credentials.MaintenanceToken); err != nil {
		return err
	}
	if err := s.advance(job, "waiting_worker", 78, "正在等待 Worker 心跳"); err != nil {
		return err
	}
	if err := executor.waitForWorker(ctx, site); err != nil {
		return err
	}
	if err := s.advance(job, "checking_health", 84, "正在检查应用就绪状态"); err != nil {
		return err
	}
	if err := executor.waitForHealth(ctx, site, true); err != nil {
		return err
	}

	if err := s.advance(job, "activating_route", 89, "正在开放分站域名路由"); err != nil {
		return err
	}
	if err := s.setRouteState(job, "activating", "", false); err != nil {
		return err
	}
	if _, err := executor.renderSite(site, credentials, image, site.DesiredVersion, true); err != nil {
		return err
	}
	if _, err := executor.loadComposeConfig(ctx, dir); err != nil {
		return err
	}
	routeEnabled = true
	if err := executor.compose(ctx, dir, "up", "-d", "--no-deps", "app"); err != nil {
		return err
	}
	if err := executor.waitForHealth(ctx, site, true); err != nil {
		return err
	}
	if err := s.advance(job, "checking_route", 94, "正在确认域名已连接到正确分站"); err != nil {
		return err
	}
	if err := executor.waitForRoute(ctx, site, false); err != nil {
		return err
	}
	if s.cfg.VerifyPublicHTTPS {
		if err := s.advance(job, "checking_https", 97, "正在检查公网 HTTPS"); err != nil {
			return err
		}
		if err := s.setRouteState(job, "verifying_https", "", false); err != nil {
			return err
		}
		if err := executor.waitForRoute(ctx, site, true); err != nil {
			return err
		}
	}

	result, _ := json.Marshal(map[string]string{
		"siteUrl": "https://" + site.Domain, "adminUsername": payload["adminUsername"],
	})
	return s.db.Transaction(func(tx *gorm.DB) error {
		claimed := tx.Exec(`UPDATE deployment_jobs SET status='completed',current_step='active',progress=100,result_json=?::jsonb,finished_at=now(),lease_until=NULL,updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, string(result), job.ID, job.WorkerID, job.LeaseVersion)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		if err := tx.Exec(`UPDATE sites SET status='active',route_status='active',route_error=NULL,route_verified_at=now(),current_version=desired_version,activated_at=COALESCE(activated_at,now()),last_heartbeat_at=now(),last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, job.SiteID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM site_secrets WHERE site_id=? AND secret_type='bootstrap_admin_password'`, job.SiteID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE provision_codes SET status='active',used_sites=LEAST(max_sites,used_sites+1),updated_at=now() WHERE bound_site_id=?`, job.SiteID).Error; err != nil {
			return err
		}
		return s.addEvent(tx, job.ID, "completed", "active", "completed", 100, "分站创建成功", "")
	})
}

func (s *Service) runSiteOperation(ctx context.Context, executor *siteExecutor, job ClaimedJob) error {
	site, err := executor.loadSite(job.SiteID)
	if err != nil {
		return err
	}
	if job.JobType == "delete" {
		return s.runDelete(ctx, executor, job, site)
	}
	credentials, err := executor.ensureCredentials(site.ID, false)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.cfg.SiteBaseDir, site.ID)
	var payload map[string]string
	_ = json.Unmarshal([]byte(job.PayloadJSON), &payload)
	result := map[string]string{}
	finalStatus := site.Status
	message := "操作已完成"

	if err := s.advance(job, "processing", 20, "正在执行站点操作"); err != nil {
		return err
	}
	switch job.JobType {
	case "start", "unfreeze":
		if err := s.setRouteState(job, "activating", "", false); err != nil {
			return err
		}
		image, err := executor.resolveImage(site.CurrentVersion)
		if err != nil {
			return err
		}
		if _, err := executor.renderSite(site, credentials, image, site.CurrentVersion, true); err != nil {
			return err
		}
		if err := executor.prepareImages(ctx, dir); err != nil {
			return err
		}
		if err := executor.compose(ctx, dir, "up", "-d", "--remove-orphans"); err != nil {
			return err
		}
		if err := executor.waitForHealth(ctx, site, true); err != nil {
			return err
		}
		if err := executor.waitForRoute(ctx, site, false); err != nil {
			_ = s.setRouteState(job, "failed", err.Error(), false)
			return err
		}
		if s.cfg.VerifyPublicHTTPS {
			if err := executor.waitForRoute(ctx, site, true); err != nil {
				_ = s.setRouteState(job, "failed", err.Error(), false)
				return err
			}
		}
		if err := s.setRouteState(job, "active", "", true); err != nil {
			return err
		}
		finalStatus = "active"
		message = "站点已启动"
	case "stop", "freeze":
		if err := executor.compose(ctx, dir, "stop"); err != nil {
			return err
		}
		if job.JobType == "freeze" {
			finalStatus = "frozen"
			message = "站点已冻结"
		} else {
			finalStatus = "stopped"
			message = "站点已停止"
		}
		if err := s.setRouteState(job, "disabled", "", false); err != nil {
			return err
		}
	case "restart":
		image, err := executor.resolveImage(site.CurrentVersion)
		if err != nil {
			return err
		}
		if _, err := executor.renderSite(site, credentials, image, site.CurrentVersion, true); err != nil {
			return err
		}
		if err := executor.prepareImages(ctx, dir); err != nil {
			return err
		}
		if err := executor.compose(ctx, dir, "up", "-d", "--remove-orphans"); err != nil {
			return err
		}
		if err := executor.waitForHealth(ctx, site, true); err != nil {
			return err
		}
		if err := executor.waitForRoute(ctx, site, false); err != nil {
			_ = s.setRouteState(job, "failed", err.Error(), false)
			return err
		}
		if s.cfg.VerifyPublicHTTPS {
			if err := executor.waitForRoute(ctx, site, true); err != nil {
				_ = s.setRouteState(job, "failed", err.Error(), false)
				return err
			}
		}
		if err := s.setRouteState(job, "active", "", true); err != nil {
			return err
		}
		finalStatus = "active"
		message = "站点已重启"
	case "backup":
		backupPath, err := executor.backup(ctx, site, credentials, job.ID)
		if err != nil {
			return err
		}
		result["backupPath"] = backupPath
		message = "数据库备份已完成"
	case "upgrade":
		return s.runUpgrade(ctx, executor, job, site, credentials, payload["targetVersion"])
	default:
		return errors.New("unsupported operation")
	}

	encodedResult, _ := json.Marshal(result)
	return s.completeOperation(job, finalStatus, message, string(encodedResult))
}

func (s *Service) runDelete(ctx context.Context, executor *siteExecutor, job ClaimedJob, site siteRuntime) error {
	dir := filepath.Join(s.cfg.SiteBaseDir, site.ID)
	if err := s.advance(job, "stopping_runtime", 15, "正在关闭站点路由和运行服务"); err != nil {
		return err
	}
	if err := s.setRouteState(job, "disabled", "", false); err != nil {
		return err
	}
	if err := executor.destroyRuntime(ctx, site, dir); err != nil {
		return err
	}
	if err := s.advance(job, "deleting_database", 50, "正在删除独立数据库和数据库账号"); err != nil {
		return err
	}
	if err := executor.destroyDatabase(ctx, site); err != nil {
		return err
	}
	if err := s.advance(job, "deleting_files", 72, "正在删除站点目录、数据卷和备份"); err != nil {
		return err
	}
	if err := executor.removeSiteDirectory(site.ID); err != nil {
		return err
	}
	if err := s.advance(job, "releasing_domain", 88, "正在清理站点配置并释放域名前缀"); err != nil {
		return err
	}
	return s.finalizeSiteDeletion(job, site)
}

func (s *Service) finalizeSiteDeletion(job ClaimedJob, site siteRuntime) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var activeClaim int64
		if err := tx.Raw(`SELECT COUNT(*) FROM deployment_jobs WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, job.ID, job.WorkerID, job.LeaseVersion).Scan(&activeClaim).Error; err != nil {
			return err
		}
		if activeClaim != 1 {
			return errors.New("job lease is no longer active")
		}
		for _, statement := range []string{
			`DELETE FROM site_secrets WHERE site_id=?`,
			`DELETE FROM report_nonces WHERE site_id=?`,
			`DELETE FROM site_heartbeats WHERE site_id=?`,
			`DELETE FROM site_metric_snapshots WHERE site_id=?`,
			`DELETE FROM site_daily_metrics WHERE site_id=?`,
			`DELETE FROM site_channel_snapshots WHERE site_id=?`,
			`DELETE FROM site_config_events WHERE site_id=?`,
			`DELETE FROM site_upgrade_records WHERE site_id=?`,
		} {
			if err := tx.Exec(statement, site.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`DELETE FROM domain_reservations WHERE full_domain=? OR provision_session_id IN (SELECT id FROM provision_sessions WHERE bound_site_id=?)`, site.Domain, site.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM provision_sessions WHERE bound_site_id=?`, site.ID).Error; err != nil {
			return err
		}
		if site.ProvisionCodeID != "" {
			if err := tx.Exec(`UPDATE provision_codes
				SET used_sites=GREATEST(used_sites-1,0),
					status=CASE WHEN expires_at IS NOT NULL AND expires_at<=now() THEN 'expired'
						WHEN GREATEST(used_sites-1,0)=0 THEN 'unused' ELSE 'active' END,
					bound_site_id=CASE WHEN bound_site_id=? THEN NULL ELSE bound_site_id END,
					reserved_at=CASE WHEN GREATEST(used_sites-1,0)=0 THEN NULL ELSE reserved_at END,
					updated_at=now()
				WHERE id=?`, site.ID, site.ProvisionCodeID).Error; err != nil {
				return err
			}
		}
		tombstoneSlug := "deleted-" + strings.ReplaceAll(site.ID, "-", "")
		tombstoneDomain := tombstoneSlug + ".invalid"
		if err := tx.Exec(`UPDATE sites SET
			provision_code_id=NULL,node_id=NULL,remark=NULL,name='已删除站点',slug=?,domain=?,logo_url=NULL,
			status='deleted',route_status='disabled',route_error=NULL,route_verified_at=NULL,
			current_version=NULL,desired_version=NULL,database_name=NULL,database_user=NULL,storage_prefix=NULL,
			app_container_name=NULL,worker_container_name=NULL,last_heartbeat_at=NULL,bootstrap_profile='{}'::jsonb,
			last_error_code=NULL,last_error_message=NULL,deleted_at=now(),updated_at=now()
			WHERE id=?`, tombstoneSlug, tombstoneDomain, site.ID).Error; err != nil {
			return err
		}
		resultJSON, _ := json.Marshal(map[string]string{"releasedDomain": site.Domain})
		claimed := tx.Exec(`UPDATE deployment_jobs SET status='completed',current_step='completed',progress=100,result_json=?::jsonb,finished_at=now(),lease_until=NULL,updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, string(resultJSON), job.ID, job.WorkerID, job.LeaseVersion)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		return s.addEvent(tx, job.ID, "completed", "completed", "completed", 100, "站点已彻底删除，域名前缀已释放", "")
	})
}

func (s *Service) runUpgrade(ctx context.Context, executor *siteExecutor, job ClaimedJob, site siteRuntime, credentials siteCredentials, targetVersion string) error {
	if targetVersion == "" || targetVersion == site.CurrentVersion {
		return errors.New("a different target version is required")
	}
	image, err := executor.resolveImage(targetVersion)
	if err != nil {
		return err
	}
	if err := s.db.Exec(`UPDATE sites SET status='upgrading',desired_version=?,last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, targetVersion, site.ID).Error; err != nil {
		return err
	}
	if err := s.advance(job, "backing_up", 30, "正在备份站点数据库"); err != nil {
		return err
	}
	backupPath, err := executor.backup(ctx, site, credentials, job.ID)
	if err != nil {
		return err
	}
	recordID := newUUID()
	if err := s.db.Exec(`INSERT INTO site_upgrade_records(id,site_id,from_version,to_version,job_id,status,backup_path,started_at) VALUES (?,?,?,?,?,'running',?,now())`, recordID, site.ID, site.CurrentVersion, targetVersion, job.ID, backupPath).Error; err != nil {
		return err
	}
	if err := s.advance(job, "pulling_image", 48, "正在拉取目标版本"); err != nil {
		return err
	}
	dir, err := executor.renderSite(site, credentials, image, targetVersion, true)
	if err != nil {
		return err
	}
	if err := executor.prepareImages(ctx, dir); err != nil {
		return err
	}
	if err := s.advance(job, "upgrading", 68, "正在升级应用和任务服务"); err != nil {
		return err
	}
	if err := executor.compose(ctx, dir, "up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	if err := s.advance(job, "checking_health", 88, "正在检查升级后的站点"); err != nil {
		return err
	}
	validationErr := executor.waitForHealth(ctx, site, true)
	if validationErr == nil {
		validationErr = executor.waitForRoute(ctx, site, false)
	}
	if validationErr == nil && s.cfg.VerifyPublicHTTPS {
		validationErr = executor.waitForRoute(ctx, site, true)
	}
	if validationErr != nil {
		oldImage, imageErr := executor.resolveImage(site.CurrentVersion)
		if imageErr == nil {
			_, _ = executor.renderSite(site, credentials, oldImage, site.CurrentVersion, true)
			if executor.prepareImages(ctx, dir) == nil {
				_ = executor.compose(ctx, dir, "up", "-d", "--remove-orphans")
			}
		}
		_ = s.db.Exec(`UPDATE site_upgrade_records SET status='rolled_back',finished_at=now() WHERE id=?`, recordID).Error
		_ = s.db.Exec(`UPDATE sites SET status='active',desired_version=current_version,updated_at=now() WHERE id=?`, site.ID).Error
		return fmt.Errorf("upgrade validation failed and application image was rolled back: %w", validationErr)
	}
	if err := s.db.Exec(`UPDATE site_upgrade_records SET status='completed',finished_at=now() WHERE id=?`, recordID).Error; err != nil {
		return err
	}
	result, _ := json.Marshal(map[string]string{"backupPath": backupPath, "fromVersion": site.CurrentVersion, "toVersion": targetVersion})
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE sites SET status='active',route_status='active',route_error=NULL,route_verified_at=now(),current_version=?,desired_version=?,last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, targetVersion, targetVersion, site.ID).Error; err != nil {
			return err
		}
		claimed := tx.Exec(`UPDATE deployment_jobs SET status='completed',current_step='completed',progress=100,result_json=?::jsonb,finished_at=now(),lease_until=NULL,updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, string(result), job.ID, job.WorkerID, job.LeaseVersion)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		return s.addEvent(tx, job.ID, "completed", "completed", "completed", 100, "站点升级完成", "")
	})
}

func (s *Service) completeOperation(job ClaimedJob, siteStatus, message, resultJSON string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if siteStatus != "" {
			if err := tx.Exec(`UPDATE sites SET status=?,last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, siteStatus, job.SiteID).Error; err != nil {
				return err
			}
		}
		claimed := tx.Exec(`UPDATE deployment_jobs SET status='completed',current_step='completed',progress=100,result_json=?::jsonb,finished_at=now(),lease_until=NULL,updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, resultJSON, job.ID, job.WorkerID, job.LeaseVersion)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		return s.addEvent(tx, job.ID, "completed", "completed", "completed", 100, message, "")
	})
}

func (s *Service) advance(job ClaimedJob, step string, progress int, message string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE deployment_jobs SET current_step=?,progress=?,lease_until=now()+interval '30 seconds',updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, step, progress, job.ID, job.WorkerID, job.LeaseVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		return s.addEvent(tx, job.ID, "progress", step, "running", progress, message, "")
	})
}

func (s *Service) setRouteState(job ClaimedJob, status, routeError string, verified bool) error {
	result := s.db.Exec(`UPDATE sites SET route_status=?,route_error=NULLIF(?,''),
		route_verified_at=CASE WHEN ? THEN now() ELSE route_verified_at END,updated_at=now()
		WHERE id=? AND EXISTS (SELECT 1 FROM deployment_jobs WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now())`,
		status, routeError, verified, job.SiteID, job.ID, job.WorkerID, job.LeaseVersion)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("job lease is no longer active")
	}
	return nil
}

func (s *Service) FailJob(job ClaimedJob, code, publicMessage, internalMessage string, retryable bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		claimed := tx.Exec(`UPDATE deployment_jobs SET status='failed',error_code=?,error_message=?,internal_error_message=?,retryable=(? AND attempt<max_attempts),finished_at=now(),lease_until=NULL,updated_at=now()
			WHERE id=? AND status='running' AND worker_id=? AND lease_version=? AND lease_until>now()`, code, publicMessage, internalMessage, retryable, job.ID, job.WorkerID, job.LeaseVersion)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("job lease is no longer active")
		}
		if job.SiteID != "" {
			statusSQL := `UPDATE sites SET last_error_code=?,last_error_message=?,updated_at=now() WHERE id=?`
			if job.JobType == "provision" {
				statusSQL = `UPDATE sites SET status='failed',route_status='failed',route_error=?,last_error_code=?,last_error_message=?,updated_at=now() WHERE id=?`
				_ = tx.Exec(`UPDATE provision_codes SET status='failed',updated_at=now() WHERE bound_site_id=?`, job.SiteID).Error
			}
			if job.JobType == "upgrade" {
				statusSQL = `UPDATE sites SET status='active',desired_version=current_version,last_error_code=?,last_error_message=?,updated_at=now() WHERE id=?`
			}
			if job.JobType == "delete" {
				statusSQL = `UPDATE sites SET status='failed',route_status='disabled',last_error_code=?,last_error_message=?,updated_at=now() WHERE id=?`
			}
			var update *gorm.DB
			if job.JobType == "provision" {
				update = tx.Exec(statusSQL, internalMessage, code, internalMessage, job.SiteID)
			} else {
				update = tx.Exec(statusSQL, code, internalMessage, job.SiteID)
			}
			if update.Error != nil {
				return update.Error
			}
		}
		return s.addEvent(tx, job.ID, "failed", "failed", "failed", 100, publicMessage, internalMessage)
	})
}

func (s *Service) RunMaintenance() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		statements := []string{
			`DELETE FROM report_nonces WHERE expires_at<=now()`,
			`DELETE FROM admin_sessions WHERE expires_at<=now()`,
			`DELETE FROM provision_sessions WHERE expires_at<=now() AND status='active'`,
			`DELETE FROM domain_reservations WHERE expires_at<=now()`,
			`UPDATE provision_codes SET status='expired',updated_at=now() WHERE expires_at<=now() AND status IN ('unused','reserved')`,
			`UPDATE provision_codes SET status='unused',reserved_at=NULL,updated_at=now() WHERE status='reserved' AND reserved_at<now()-interval '30 minutes'`,
			`UPDATE sites SET status='warning',updated_at=now() WHERE status='active' AND last_heartbeat_at<now()-interval '60 seconds'`,
			`UPDATE sites SET status='offline',updated_at=now() WHERE status IN ('active','warning') AND last_heartbeat_at<now()-interval '120 seconds'`,
			`DELETE FROM site_heartbeats WHERE received_at<now()-interval '30 days'`,
			`DELETE FROM site_metric_snapshots WHERE received_at<now()-interval '180 days'`,
			`DELETE FROM site_channel_snapshots WHERE received_at<now()-interval '90 days'`,
			`DELETE FROM admin_login_attempts WHERE created_at<now()-interval '30 days'`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) UpdateAgentHeartbeat() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dockerVersion, dockerErr := osCommandRunner{}.Run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	lastError := ""
	status := "online"
	if dockerErr != nil {
		lastError = "docker daemon unavailable"
		status = "warning"
	}
	return s.db.Exec(`INSERT INTO deployment_nodes(name,status,agent_version,docker_version,cpu_total,last_error,last_heartbeat_at,site_count)
		VALUES (?,?,'local',?,?,NULLIF(?,''),now(),(SELECT count(*) FROM sites WHERE deleted_at IS NULL))
		ON CONFLICT(name) DO UPDATE SET status='online',agent_version=EXCLUDED.agent_version,
		docker_version=EXCLUDED.docker_version,cpu_total=EXCLUDED.cpu_total,last_error=EXCLUDED.last_error,
		last_heartbeat_at=EXCLUDED.last_heartbeat_at,site_count=EXCLUDED.site_count,updated_at=now()`, s.cfg.AgentID, status, strings.TrimSpace(dockerVersion), runtime.NumCPU(), lastError).Error
}

func IsNoJob(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func newUUID() string {
	value, _ := randomHex(16)
	return fmt.Sprintf("%s-%s-%s-%s-%s", value[:8], value[8:12], value[12:16], value[16:20], value[20:32])
}

func cleanupTemporaryFile(path string) { _ = os.Remove(path) }

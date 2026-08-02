package control

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	codeUnused       = "unused"
	codeReserved     = "reserved"
	codeProvisioning = "provisioning"
	codeActive       = "active"
	codeFailed       = "failed"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,30}[a-z0-9])?$`)
var reservedPrefixes = map[string]bool{"open": true, "www": true, "api": true, "admin": true, "control": true, "status": true, "static": true, "cdn": true, "mail": true, "docs": true, "support": true, "download": true, "assets": true, "health": true, "zhimeng": true}

type Service struct {
	cfg config.Config
	db  *gorm.DB
}

func NewService(cfg config.Config, db *gorm.DB) *Service { return &Service{cfg: cfg, db: db} }

type Code struct {
	ID             string     `json:"id"`
	Prefix         string     `json:"prefix"`
	Remark         string     `json:"remark"`
	Status         string     `json:"status"`
	InitialVersion string     `json:"initialVersion"`
	MaxSites       int        `json:"maxSites"`
	UsedSites      int        `json:"usedSites"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	CreatedAt      time.Time  `json:"createdAt"`
}
type Session struct {
	ID          string             `json:"id"`
	ExpiresAt   time.Time          `json:"expiresAt"`
	Reservation *DomainReservation `json:"reservation,omitempty"`
}
type DomainReservation struct {
	ID        string    `json:"reservationId"`
	Domain    string    `json:"domain"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type Job struct {
	ID           string    `json:"id"`
	SiteID       string    `json:"siteId"`
	JobType      string    `json:"jobType"`
	Status       string    `json:"status"`
	CurrentStep  string    `json:"currentStep"`
	ErrorCode    string    `json:"errorCode"`
	ErrorMessage string    `json:"errorMessage"`
	ResultJSON   string    `json:"resultJSON"`
	Progress     int       `json:"progress"`
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"maxAttempts"`
	Retryable    bool      `json:"retryable"`
	WorkerID     string    `json:"workerId,omitempty"`
	LeaseVersion int64     `json:"leaseVersion,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type JobEvent struct {
	ID              string    `json:"id"`
	Sequence        int64     `json:"sequence"`
	EventType       string    `json:"eventType"`
	Step            string    `json:"step"`
	Status          string    `json:"status"`
	Progress        int       `json:"progress"`
	PublicMessage   string    `json:"publicMessage"`
	InternalMessage string    `json:"-"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (s *Service) CreateCodes(remark string, quantity, maxSites int, expiresAt *time.Time, initialVersion string) ([]string, error) {
	if quantity < 1 || quantity > 1000 || maxSites < 1 || maxSites > 100 {
		return nil, errors.New("invalid code quantity")
	}
	codes := make([]string, 0, quantity)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < quantity; i++ {
			plain, err := newCode()
			if err != nil {
				return err
			}
			prefix := plain[:9]
			if err := tx.Exec(`INSERT INTO provision_codes (code_prefix, code_hash, remark, max_sites, expires_at, initial_version) VALUES (?, ?, ?, ?, ?, ?)`, prefix, s.hashCode(plain), remark, maxSites, expiresAt, initialVersion).Error; err != nil {
				return err
			}
			codes = append(codes, plain)
		}
		return nil
	})
	return codes, err
}

func (s *Service) ListCodes(page, size int, status, query string) ([]Code, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	base := s.db.Table("provision_codes")
	if status != "" {
		base = base.Where("status = ?", status)
	}
	if query != "" {
		base = base.Where("remark ILIKE ? OR code_prefix ILIKE ?", "%"+query+"%", "%"+query+"%")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Code
	err := base.Select("id, code_prefix as prefix, remark, status, initial_version, max_sites, used_sites, expires_at, created_at").Order("created_at DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	return rows, total, err
}

func (s *Service) RevokeCode(id string) error {
	result := s.db.Exec(`UPDATE provision_codes SET status = 'revoked', updated_at = now() WHERE id = ? AND status IN ('unused','reserved','failed')`, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("code cannot be revoked")
	}
	return nil
}
func (s *Service) DeleteCode(id string) error {
	result := s.db.Exec(`DELETE FROM provision_codes WHERE id = ? AND status = 'unused'`, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("only unused code can be deleted")
	}
	return nil
}

func (s *Service) VerifyCode(plain, ip string) (string, Session, *Job, error) {
	var row struct {
		ID, Status  string
		ExpiresAt   *time.Time
		BoundSiteID *string
	}
	if err := s.db.Raw(`SELECT id, status, expires_at, bound_site_id FROM provision_codes WHERE code_hash = ?`, s.hashCode(normalizeCode(plain))).Scan(&row).Error; err != nil {
		return "", Session{}, nil, err
	}
	if row.ID == "" || (row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now().UTC())) {
		return "", Session{}, nil, errors.New("invalid or expired code")
	}
	if row.Status == codeReserved {
		token, err := security.RandomToken(32)
		if err != nil {
			return "", Session{}, nil, err
		}
		session := Session{}
		err = s.db.Transaction(func(tx *gorm.DB) error {
			var current struct {
				ID                   string
				ExpiresAt            time.Time
				ReservationID        string
				ReservationDomain    string
				ReservationExpiresAt *time.Time
			}
			if err := tx.Raw(`SELECT ps.id,ps.expires_at,
				COALESCE(dr.id::text,'') reservation_id,
				COALESCE(dr.full_domain,'') reservation_domain,
				dr.expires_at reservation_expires_at
				FROM provision_sessions ps
				LEFT JOIN domain_reservations dr ON dr.id=ps.reservation_id
					AND dr.expires_at>now() AND dr.consumed_at IS NULL
				WHERE ps.provision_code_id=? AND ps.status='active' AND ps.expires_at>now()
				ORDER BY ps.created_at DESC LIMIT 1 FOR UPDATE OF ps`, row.ID).Scan(&current).Error; err != nil {
				return err
			}
			if current.ID == "" {
				current.ID = uuid.NewString()
				current.ExpiresAt = time.Now().UTC().Add(30 * time.Minute)
				if err := tx.Exec(`INSERT INTO provision_sessions(id,provision_code_id,token_hash,status,expires_at,created_ip)
					VALUES (?,?,?,'active',?,NULLIF(?,'')::inet)`, current.ID, row.ID, security.SHA256Hex(token), current.ExpiresAt, ip).Error; err != nil {
					return err
				}
				if err := tx.Exec(`UPDATE provision_codes SET reserved_at=now(),updated_at=now() WHERE id=? AND status='reserved'`, row.ID).Error; err != nil {
					return err
				}
			} else {
				result := tx.Exec(`UPDATE provision_sessions SET token_hash=?,created_ip=NULLIF(?,'')::inet,updated_at=now()
					WHERE id=? AND status='active' AND expires_at>now()`, security.SHA256Hex(token), ip, current.ID)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return errors.New("provision session expired")
				}
			}
			session = Session{ID: current.ID, ExpiresAt: current.ExpiresAt}
			if current.ReservationID != "" && current.ReservationExpiresAt != nil {
				session.Reservation = &DomainReservation{
					ID: current.ReservationID, Domain: current.ReservationDomain, ExpiresAt: *current.ReservationExpiresAt,
				}
			}
			return nil
		})
		return token, session, nil, err
	}
	if row.Status == codeActive || row.Status == codeProvisioning || row.Status == codeFailed {
		var job Job
		if row.BoundSiteID != nil {
			_ = s.db.Raw(`SELECT id, site_id, job_type, status, current_step, progress, attempt, max_attempts, retryable, created_at, updated_at FROM deployment_jobs WHERE site_id = ? ORDER BY created_at DESC LIMIT 1`, *row.BoundSiteID).Scan(&job).Error
		}
		token, err := security.RandomToken(32)
		if err != nil {
			return "", Session{}, nil, err
		}
		session := Session{ID: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)}
		var boundJob any
		if job.ID != "" {
			boundJob = job.ID
		}
		err = s.db.Exec(`INSERT INTO provision_sessions(id,provision_code_id,token_hash,status,expires_at,created_ip,bound_site_id,bound_job_id) VALUES (?,?,?,'completed',?,NULLIF(?,'')::inet,?,?)`, session.ID, row.ID, security.SHA256Hex(token), session.ExpiresAt, ip, row.BoundSiteID, boundJob).Error
		return token, session, &job, err
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return "", Session{}, nil, err
	}
	session := Session{ID: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE provision_codes SET status = ?, reserved_at = now(), updated_at = now() WHERE id = ? AND status = 'unused'`, codeReserved, row.ID)
		if result.Error != nil || result.RowsAffected != 1 {
			return errors.New("code is currently in use")
		}
		return tx.Exec(`INSERT INTO provision_sessions (id, provision_code_id, token_hash, expires_at, created_ip) VALUES (?, ?, ?, ?, NULLIF(?, '')::inet)`, session.ID, row.ID, security.SHA256Hex(token), session.ExpiresAt, ip).Error
	})
	return token, session, nil, err
}

func (s *Service) ReserveDomain(token, prefix string) (DomainReservation, error) {
	sessionID, err := s.sessionID(token)
	if err != nil {
		return DomainReservation{}, err
	}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if !slugPattern.MatchString(prefix) || reservedPrefixes[prefix] {
		return DomainReservation{}, errors.New("invalid domain prefix")
	}
	domain := prefix + "." + strings.ToLower(strings.TrimSpace(s.cfg.RootDomain))
	expires := time.Now().UTC().Add(20 * time.Minute)
	reservation := DomainReservation{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var occupied int64
		if err := tx.Raw(`SELECT COUNT(*) FROM sites WHERE domain = ? AND deleted_at IS NULL`, domain).Scan(&occupied).Error; err != nil {
			return err
		}
		if occupied > 0 {
			return errors.New("domain is not available")
		}
		if err := tx.Raw(`INSERT INTO domain_reservations (prefix, full_domain, provision_session_id, expires_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (full_domain) DO UPDATE
			SET prefix=EXCLUDED.prefix, provision_session_id=EXCLUDED.provision_session_id,
				expires_at=EXCLUDED.expires_at, consumed_at=NULL
			WHERE domain_reservations.provision_session_id=EXCLUDED.provision_session_id
				OR (domain_reservations.expires_at<=now() AND domain_reservations.consumed_at IS NULL)
			RETURNING id, full_domain AS domain, expires_at`, prefix, domain, sessionID, expires).Scan(&reservation).Error; err != nil {
			return err
		}
		if reservation.ID == "" {
			return errors.New("domain is not available")
		}
		result := tx.Exec(`UPDATE provision_sessions SET reservation_id=?,updated_at=now() WHERE id=? AND status='active'`, reservation.ID, sessionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("provision session expired")
		}
		return nil
	})
	return reservation, err
}

type CreateSiteInput struct {
	Token, ReservationID, Prefix, Name, AdminUsername, AdminPassword string
	SiteProfile                                                      siteBootstrapProfile
}

func (s *Service) CreateProvisionJob(in CreateSiteInput) (Job, error) {
	if len(strings.TrimSpace(in.Name)) < 2 || len(in.Name) > 128 {
		return Job{}, errors.New("invalid site name")
	}
	if len(in.AdminUsername) < 4 || len(in.AdminUsername) > 32 || len(in.AdminPassword) < 10 || !hasLetterAndNumber(in.AdminPassword) {
		return Job{}, errors.New("invalid administrator credentials")
	}
	profile, err := normalizeSiteBootstrapProfile(in.SiteProfile)
	if err != nil {
		return Job{}, err
	}
	if !s.validLogoURL(profile.LogoURL) || !s.validLogoURL(profile.IconURL) {
		return Job{}, errors.New("invalid uploaded site image URL")
	}
	prefix := strings.ToLower(strings.TrimSpace(in.Prefix))
	if !slugPattern.MatchString(prefix) || reservedPrefixes[prefix] || strings.TrimSpace(in.ReservationID) == "" {
		return Job{}, errors.New("valid domain reservation is required")
	}
	job := Job{ID: uuid.NewString(), JobType: "provision", Status: "pending", CurrentStep: "pending", Progress: 0, Attempt: 0, MaxAttempts: 3, Retryable: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	siteID := uuid.NewString()
	job.SiteID = siteID
	payload, _ := json.Marshal(map[string]string{"adminUsername": in.AdminUsername})
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return Job{}, err
	}
	protectedPassword, err := security.Seal(in.AdminPassword, s.cfg.MasterEncryptionKey)
	if err != nil {
		return Job{}, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var session struct {
			ID, CodeID, ReservationID string
			BoundJobID                *string
		}
		if err := tx.Raw(`SELECT id, provision_code_id AS code_id, COALESCE(reservation_id::text,'') AS reservation_id, bound_job_id
			FROM provision_sessions WHERE token_hash=? AND status IN ('active','completed') AND expires_at>now() FOR UPDATE`, security.SHA256Hex(in.Token)).Scan(&session).Error; err != nil || session.CodeID == "" {
			return errors.New("provision session not found")
		}
		if session.BoundJobID != nil && *session.BoundJobID != "" {
			existing, err := getJobWithDB(tx, *session.BoundJobID)
			if err != nil {
				return err
			}
			job = existing
			return nil
		}
		if session.ReservationID != in.ReservationID {
			return errors.New("domain reservation does not match this session")
		}
		var reservation struct{ ID, Prefix, Domain string }
		if err := tx.Raw(`SELECT id,prefix,full_domain AS domain FROM domain_reservations
			WHERE id=? AND provision_session_id=? AND consumed_at IS NULL AND expires_at>now() FOR UPDATE`, in.ReservationID, session.ID).Scan(&reservation).Error; err != nil || reservation.ID == "" {
			return errors.New("domain reservation expired")
		}
		if reservation.Prefix != prefix {
			return errors.New("domain reservation does not match requested prefix")
		}
		if err := tx.Exec(`INSERT INTO sites (id, provision_code_id, remark, name, slug, domain, logo_url, bootstrap_profile, status, route_status, current_version, desired_version, database_name, database_user, storage_prefix, app_container_name, worker_container_name) SELECT ?, id, remark, ?, ?, ?, ?, ?::jsonb, 'pending', 'disabled', COALESCE(initial_version, 'latest'), COALESCE(initial_version, 'latest'), ?, ?, ?, ?, ? FROM provision_codes WHERE id = ?`, siteID, in.Name, prefix, reservation.Domain, profile.LogoURL, string(profileJSON), "site_"+strings.ReplaceAll(siteID, "-", ""), "user_"+strings.ReplaceAll(siteID, "-", ""), "sites/"+siteID, "site-"+siteID+"-app", "site-"+siteID+"-worker", session.CodeID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO site_secrets (site_id, secret_type, ciphertext) VALUES (?, 'bootstrap_admin_password', ?)`, siteID, protectedPassword).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO deployment_jobs (id, site_id, job_type, status, current_step, progress, payload_json) VALUES (?, ?, 'provision', 'pending', 'pending', 0, ?::jsonb)`, job.ID, siteID, string(payload)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE provision_codes SET status = 'provisioning', bound_site_id = ?, updated_at = now() WHERE id = ?`, siteID, session.CodeID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE domain_reservations SET consumed_at=now() WHERE id=? AND consumed_at IS NULL`, reservation.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE provision_sessions SET status='completed',bound_site_id=?,bound_job_id=?,updated_at=now() WHERE id=?`, siteID, job.ID, session.ID).Error; err != nil {
			return err
		}
		return s.addEvent(tx, job.ID, "progress", "pending", "pending", 0, "Provisioning request accepted", "")
	})
	return job, err
}

func (s *Service) GetJob(id string) (Job, error) {
	return getJobWithDB(s.db, id)
}

func getJobWithDB(db *gorm.DB, id string) (Job, error) {
	var job Job
	err := db.Raw(`SELECT id, site_id, job_type, status, current_step, progress, attempt, max_attempts, retryable, error_code, error_message, result_json::text AS result_json, COALESCE(worker_id,'') worker_id, lease_version, created_at, updated_at FROM deployment_jobs WHERE id = ?`, id).Scan(&job).Error
	if err != nil || job.ID == "" {
		return Job{}, gorm.ErrRecordNotFound
	}
	return job, nil
}
func (s *Service) CanAccessPublicJob(token, jobID string) bool {
	var count int64
	err := s.db.Raw(`SELECT COUNT(*) FROM deployment_jobs j JOIN sites s ON s.id=j.site_id JOIN provision_codes pc ON pc.id=s.provision_code_id JOIN provision_sessions ps ON ps.provision_code_id=pc.id WHERE j.id=? AND ps.token_hash=? AND ps.status IN ('active','completed') AND ps.expires_at>now() AND (ps.bound_job_id=j.id OR ps.bound_job_id IS NULL)`, jobID, security.SHA256Hex(token)).Scan(&count).Error
	return err == nil && count == 1
}
func (s *Service) JobEvents(jobID string, after int64) ([]JobEvent, error) {
	var rows []JobEvent
	err := s.db.Raw(`SELECT id, sequence, event_type, step, status, progress, public_message, internal_message, created_at FROM deployment_job_events WHERE job_id = ? AND sequence > ? ORDER BY sequence`, jobID, after).Scan(&rows).Error
	return rows, err
}
func (s *Service) RetryJob(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var job Job
		if err := tx.Raw(`SELECT id, site_id, job_type, status, current_step, progress, attempt, max_attempts, retryable, created_at, updated_at FROM deployment_jobs WHERE id = ? FOR UPDATE`, id).Scan(&job).Error; err != nil || job.ID == "" {
			return gorm.ErrRecordNotFound
		}
		if !job.Retryable || job.Attempt >= job.MaxAttempts || (job.Status != "failed" && job.Status != "manual_intervention") {
			return errors.New("job cannot be retried")
		}
		if err := tx.Exec(`UPDATE deployment_jobs SET status='pending',error_code=NULL,error_message=NULL,internal_error_message=NULL,worker_id=NULL,lease_until=NULL,updated_at=now() WHERE id=?`, id).Error; err != nil {
			return err
		}
		return s.addEvent(tx, id, "progress", job.CurrentStep, "pending", job.Progress, "Retry queued", "")
	})
}

func (s *Service) ListSites(page, size int, status, query string) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.db.Table("sites s").Where("s.deleted_at IS NULL")
	if status != "" {
		q = q.Where("s.status=?", status)
	}
	if query != "" {
		q = q.Where("s.name ILIKE ? OR s.domain ILIKE ?", "%"+query+"%", "%"+query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []map[string]interface{}
	err := q.Select(`s.id,s.remark,s.name,s.domain,s.status,s.route_status,s.route_verified_at,s.current_version,s.desired_version,s.last_heartbeat_at,s.created_at,COALESCE(m.users_total,0) users_total,COALESCE(m.calls_today,0) calls_today,COALESCE(m.calls_7d,0) calls_7d`).Joins("LEFT JOIN LATERAL (SELECT * FROM site_metric_snapshots WHERE site_id=s.id ORDER BY received_at DESC LIMIT 1) m ON true").Order("s.created_at DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	return rows, total, err
}
func (s *Service) SiteDetail(id string) (map[string]interface{}, error) {
	var row map[string]interface{}
	err := s.db.Raw(`SELECT s.*, n.name AS node_name, COALESCE(m.users_total,0) users_total, COALESCE(m.calls_today,0) calls_today, COALESCE(m.calls_7d,0) calls_7d, COALESCE(m.calls_lifetime,0) calls_lifetime, COALESCE(m.success_rate,0) success_rate FROM sites s LEFT JOIN deployment_nodes n ON n.id=s.node_id LEFT JOIN LATERAL (SELECT * FROM site_metric_snapshots WHERE site_id=s.id ORDER BY received_at DESC LIMIT 1) m ON true WHERE s.id=? AND s.deleted_at IS NULL`, id).Scan(&row).Error
	if err != nil || row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return row, nil
}
func (s *Service) SiteMetrics(id string) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	err := s.db.Raw(`SELECT * FROM site_metric_snapshots WHERE site_id=? ORDER BY received_at DESC LIMIT 90`, id).Scan(&rows).Error
	return rows, err
}
func (s *Service) SiteChannels(id string) (map[string]interface{}, error) {
	var row map[string]interface{}
	err := s.db.Raw(`SELECT * FROM site_channel_snapshots WHERE site_id=? ORDER BY received_at DESC LIMIT 1`, id).Scan(&row).Error
	if err != nil || row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return row, nil
}
func (s *Service) CreateSiteJob(siteID, jobType, targetVersion, confirmation string) (Job, error) {
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "freeze": true, "unfreeze": true, "backup": true, "upgrade": true, "delete": true}
	if !allowed[jobType] {
		return Job{}, errors.New("unsupported operation")
	}
	job := Job{ID: uuid.NewString(), SiteID: siteID, JobType: jobType, Status: "pending", CurrentStep: "pending", MaxAttempts: 3, Retryable: true}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var target struct {
			ID     string
			Domain string
			Status string
		}
		if err := tx.Raw(`SELECT id,domain,status FROM sites WHERE id=? AND deleted_at IS NULL FOR UPDATE`, siteID).Scan(&target).Error; err != nil {
			return err
		}
		if target.ID == "" {
			return gorm.ErrRecordNotFound
		}
		if jobType == "upgrade" && targetVersion == "" {
			return errors.New("target version is required")
		}
		if jobType == "delete" && strings.TrimSpace(confirmation) != target.Domain {
			return errors.New("请输入完整域名确认彻底删除")
		}
		var activeJobs int64
		if err := tx.Raw(`SELECT COUNT(*) FROM deployment_jobs WHERE site_id=? AND status IN ('pending','running')`, siteID).Scan(&activeJobs).Error; err != nil {
			return err
		}
		if activeJobs > 0 {
			return errors.New("该站点已有任务正在执行")
		}
		payload, _ := json.Marshal(map[string]string{"targetVersion": targetVersion, "previousStatus": target.Status})
		if err := tx.Exec(`INSERT INTO deployment_jobs (id,site_id,job_type,status,current_step,payload_json) VALUES (?,?,?,'pending','pending',?::jsonb)`, job.ID, siteID, jobType, string(payload)).Error; err != nil {
			return err
		}
		if jobType == "delete" {
			if err := tx.Exec(`UPDATE sites SET status='deleting',route_status='disabling',last_error_code=NULL,last_error_message=NULL,updated_at=now() WHERE id=?`, siteID).Error; err != nil {
				return err
			}
		}
		return s.addEvent(tx, job.ID, "progress", "pending", "pending", 0, "操作已进入队列", "")
	})
	return job, err
}

func (s *Service) dashboardSummary() (map[string]interface{}, error) {
	var row map[string]interface{}
	err := s.db.Raw(`SELECT COUNT(*) sites_total, COUNT(*) FILTER (WHERE status='active' AND route_status IN ('active','unverified')) sites_online, COUNT(*) FILTER (WHERE status IN ('warning','offline','failed') OR route_status='failed') sites_abnormal, COALESCE(SUM(m.calls_today),0) calls_today, COALESCE(SUM(m.calls_7d),0) calls_7d, COALESCE(SUM(m.users_total),0) users_total, COALESCE(ROUND(AVG(m.success_rate),2),0) success_rate, COUNT(*) FILTER (WHERE desired_version IS DISTINCT FROM current_version) pending_upgrades, (SELECT COUNT(*) FROM deployment_jobs WHERE status IN ('pending','running')) jobs_running, (SELECT COUNT(*) FROM provision_codes WHERE status='unused') codes_unused FROM sites s LEFT JOIN LATERAL (SELECT * FROM site_metric_snapshots WHERE site_id=s.id ORDER BY received_at DESC LIMIT 1) m ON true WHERE s.deleted_at IS NULL`).Scan(&row).Error
	return row, err
}
func (s *Service) ListJobs(page, size int, status, jobType, siteID, query string) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.db.Table("deployment_jobs").Joins("LEFT JOIN sites ON sites.id=deployment_jobs.site_id")
	if status != "" {
		q = q.Where("deployment_jobs.status=?", status)
	}
	if jobType != "" {
		q = q.Where("deployment_jobs.job_type=?", jobType)
	}
	if siteID != "" {
		q = q.Where("deployment_jobs.site_id=?", siteID)
	}
	if query != "" {
		q = q.Where("sites.name ILIKE ? OR sites.domain ILIKE ? OR deployment_jobs.id::text ILIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []map[string]interface{}
	err := q.Select("deployment_jobs.*,sites.name site_name,sites.domain").Order("deployment_jobs.created_at DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	return rows, total, err
}
func (s *Service) ListVersions() ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	err := s.db.Table("release_versions").Order("created_at DESC").Find(&rows).Error
	return rows, err
}
func (s *Service) CreateVersion(version, image, channel, notes, migrationVersion, minUpgradeVersion string, forceUpgrade bool) (string, error) {
	id := uuid.NewString()
	version = strings.TrimSpace(version)
	image = strings.TrimSpace(image)
	if version == "" || image == "" || strings.ContainsAny(image, " \t\r\n") {
		return "", errors.New("version and valid image are required")
	}
	if !map[string]bool{"stable": true, "beta": true, "canary": true}[channel] {
		return "", errors.New("invalid release channel")
	}
	return id, s.db.Exec(`INSERT INTO release_versions (id,version,image,channel,release_notes,migration_version,min_upgrade_version,force_upgrade) VALUES (?,?,?,?,?,?,?,?)`, id, version, image, channel, notes, migrationVersion, minUpgradeVersion, forceUpgrade).Error
}
func (s *Service) PublishVersion(id string) error {
	return s.db.Exec(`UPDATE release_versions SET status='published',published_at=now(),updated_at=now() WHERE id=?`, id).Error
}
func (s *Service) ListNodes() ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	err := s.db.Table("deployment_nodes").Order("name").Find(&rows).Error
	return rows, err
}
func (s *Service) ListAudit(page, size int, action, query string) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.db.Table("audit_logs a").Joins("LEFT JOIN admin_users u ON u.id=a.admin_user_id")
	if action != "" {
		q = q.Where("a.action LIKE ?", action+"%")
	}
	if query != "" {
		q = q.Where("u.username ILIKE ? OR a.target_id ILIKE ? OR a.ip::text ILIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []map[string]interface{}
	err := q.Select("a.*,u.username admin_username").Order("a.created_at DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	return rows, total, err
}

func (s *Service) sessionID(token string) (string, error) {
	var id string
	err := s.db.Raw(`SELECT id FROM provision_sessions WHERE token_hash=? AND status='active' AND expires_at>now()`, security.SHA256Hex(token)).Scan(&id).Error
	if err != nil || id == "" {
		return "", errors.New("provision session expired")
	}
	return id, nil
}
func (s *Service) hashCode(code string) string {
	secret := s.cfg.CardHashSecret
	if secret == "" {
		secret = s.cfg.SessionSecret
	}
	return security.HMACSHA256Base64(normalizeCode(code), secret)
}
func normalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }
func newCode() (string, error) {
	token, err := security.RandomToken(18)
	if err != nil {
		return "", err
	}
	token = strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(token))
	if len(token) < 16 {
		return newCode()
	}
	return fmt.Sprintf("SITE-%s-%s-%s-%s", token[0:4], token[4:8], token[8:12], token[12:16]), nil
}
func hasLetterAndNumber(v string) bool {
	var l, n bool
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			l = true
		}
		if r >= '0' && r <= '9' {
			n = true
		}
	}
	return l && n
}

func (s *Service) validLogoURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.IsAbs() {
		base, err := url.Parse(strings.TrimRight(s.cfg.PublicBaseURL, "/"))
		return err == nil && parsed.Scheme == base.Scheme && parsed.Host == base.Host && strings.HasPrefix(parsed.Path, "/uploads/")
	}
	return strings.HasPrefix(parsed.Path, "/uploads/") && parsed.Host == ""
}
func (s *Service) addEvent(tx *gorm.DB, jobID, eventType, step, status string, progress int, publicMessage, internalMessage string) error {
	return tx.Exec(`INSERT INTO deployment_job_events (job_id,sequence,event_type,step,status,progress,public_message,internal_message) VALUES (?,(SELECT COALESCE(MAX(sequence),0)+1 FROM deployment_job_events WHERE job_id=?),?,?,?,?,?,?)`, jobID, jobID, eventType, step, status, progress, publicMessage, internalMessage).Error
}
func ConstantEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

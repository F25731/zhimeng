package control

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/F25731/zhimeng/backend/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// VerifySiteSignature validates the documented per-site HMAC and consumes the nonce atomically.
func (h *Handler) VerifySiteSignature(c *gin.Context) bool {
	siteID := strings.TrimSpace(c.GetHeader("X-Site-Id"))
	signature := strings.TrimSpace(c.GetHeader("X-Signature"))
	timestamp := strings.TrimSpace(c.GetHeader("X-Timestamp"))
	nonce := strings.TrimSpace(c.GetHeader("X-Nonce"))
	if siteID == "" || signature == "" || timestamp == "" || nonce == "" || len(nonce) > 128 {
		httpx.Error(c, http.StatusUnauthorized, 40110, "site signature required")
		return false
	}

	unixTime, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unixTime, 0)).Abs() > 5*time.Minute {
		httpx.Error(c, http.StatusUnauthorized, 40111, "expired site signature")
		return false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		httpx.Error(c, http.StatusBadRequest, 40000, "invalid request")
		return false
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var row struct{ Ciphertext, Status string }
	err = h.service.db.Raw(`SELECT ss.ciphertext, s.status FROM site_secrets ss JOIN sites s ON s.id=ss.site_id WHERE ss.site_id=? AND ss.secret_type='report_token' AND s.deleted_at IS NULL`, siteID).Scan(&row).Error
	if err != nil || row.Ciphertext == "" || row.Status == "frozen" {
		httpx.Error(c, http.StatusUnauthorized, 40110, "invalid site signature")
		return false
	}
	token, err := security.Open(row.Ciphertext, h.service.cfg.MasterEncryptionKey)
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, 40110, "invalid site signature")
		return false
	}
	bodyHash := sha256.Sum256(body)
	signingString := siteSigningString(siteID, timestamp, nonce, c.Request.Method, c.Request.URL.Path, bodyHash[:])
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(signingString))
	if !ConstantEqual(hex.EncodeToString(mac.Sum(nil)), strings.ToLower(signature)) {
		httpx.Error(c, http.StatusUnauthorized, 40110, "invalid site signature")
		return false
	}

	result := h.service.db.Exec(`INSERT INTO report_nonces(site_id,nonce,expires_at) VALUES (?,?,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, siteID, nonce)
	if result.Error != nil || result.RowsAffected != 1 {
		httpx.Error(c, http.StatusUnauthorized, 40112, "replayed site report")
		return false
	}
	c.Set("report_site_id", siteID)
	return true
}

func siteSigningString(siteID, timestamp, nonce, method, path string, bodyHash []byte) string {
	return strings.Join([]string{siteID, timestamp, nonce, method, path, hex.EncodeToString(bodyHash)}, "\n")
}

func (h *Handler) Heartbeat(c *gin.Context) {
	if !h.VerifySiteSignature(c) {
		return
	}
	var req struct {
		Version        string    `json:"version"`
		AppStatus      string    `json:"appStatus"`
		WorkerStatus   string    `json:"workerStatus"`
		DatabaseStatus string    `json:"databaseStatus"`
		RunningTasks   int       `json:"runningTasks"`
		ReportedAt     time.Time `json:"reportedAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	if req.ReportedAt.IsZero() {
		req.ReportedAt = time.Now().UTC()
	}
	siteID := c.MustGet("report_site_id").(string)
	if err := h.service.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO site_heartbeats (site_id,version,app_status,worker_status,database_status,running_tasks,reported_at) VALUES (?,?,?,?,?,?,?)`, siteID, req.Version, req.AppStatus, req.WorkerStatus, req.DatabaseStatus, req.RunningTasks, req.ReportedAt).Error; err != nil {
			return err
		}
		status := "active"
		if req.AppStatus != "healthy" || req.WorkerStatus != "healthy" || req.DatabaseStatus != "healthy" {
			status = "warning"
		}
		return tx.Exec(`UPDATE sites SET current_version=COALESCE(NULLIF(?,''),current_version),status=?,last_heartbeat_at=now(),updated_at=now() WHERE id=?`, req.Version, status, siteID).Error
	}); err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) Metrics(c *gin.Context) {
	if !h.VerifySiteSignature(c) {
		return
	}
	var req struct {
		UsersTotal        int64           `json:"usersTotal"`
		UsersActive       int64           `json:"usersActive"`
		CallsToday        int64           `json:"callsToday"`
		Calls7d           int64           `json:"calls7d"`
		CallsLifetime     int64           `json:"callsLifetime"`
		Success7d         int64           `json:"success7d"`
		Failed7d          int64           `json:"failed7d"`
		ActiveUsers7d     int64           `json:"activeUsers7d"`
		SuccessRate       float64         `json:"successRate"`
		ModelDistribution json.RawMessage `json:"modelDistribution"`
		ReportedAt        time.Time       `json:"reportedAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	if req.ReportedAt.IsZero() {
		req.ReportedAt = time.Now().UTC()
	}
	if len(req.ModelDistribution) == 0 {
		req.ModelDistribution = []byte("{}")
	}
	siteID := c.MustGet("report_site_id").(string)
	err := h.service.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO site_metric_snapshots (site_id,users_total,users_active,calls_today,calls_7d,calls_lifetime,success_7d,failed_7d,active_users_7d,success_rate,model_distribution_json,reported_at) VALUES (?,?,?,?,?,?,?,?,?,?,?::jsonb,?)`, siteID, req.UsersTotal, req.UsersActive, req.CallsToday, req.Calls7d, req.CallsLifetime, req.Success7d, req.Failed7d, req.ActiveUsers7d, req.SuccessRate, string(req.ModelDistribution), req.ReportedAt).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO site_daily_metrics(site_id,metric_date,users_total,users_active,calls_total,calls_success,calls_failed,model_distribution_json) VALUES (?,?,?, ?,?,?,?,?::jsonb) ON CONFLICT(site_id,metric_date) DO UPDATE SET users_total=EXCLUDED.users_total,users_active=EXCLUDED.users_active,calls_total=EXCLUDED.calls_total,calls_success=EXCLUDED.calls_success,calls_failed=EXCLUDED.calls_failed,model_distribution_json=EXCLUDED.model_distribution_json,updated_at=now()`, siteID, req.ReportedAt.UTC().Format("2006-01-02"), req.UsersTotal, req.UsersActive, req.CallsToday, req.Success7d, req.Failed7d, string(req.ModelDistribution)).Error
	})
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) Channels(c *gin.Context) {
	if !h.VerifySiteSignature(c) {
		return
	}
	var req struct {
		ConfigRevision  string          `json:"configRevision"`
		Channels        json.RawMessage `json:"channels"`
		HealthyChannels int             `json:"healthyChannels"`
		TotalChannels   int             `json:"totalChannels"`
		ReportedAt      time.Time       `json:"reportedAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	if len(req.Channels) == 0 {
		req.Channels = []byte("[]")
	}
	if req.ReportedAt.IsZero() {
		req.ReportedAt = time.Now().UTC()
	}
	siteID := c.MustGet("report_site_id").(string)
	err := h.service.db.Transaction(func(tx *gorm.DB) error {
		var previous string
		_ = tx.Raw(`SELECT config_revision FROM site_channel_snapshots WHERE site_id=? ORDER BY received_at DESC LIMIT 1`, siteID).Scan(&previous).Error
		if err := tx.Exec(`INSERT INTO site_channel_snapshots (site_id,config_revision,channels_json,healthy_channels,total_channels,reported_at) VALUES (?,?,?::jsonb,?,?,?)`, siteID, req.ConfigRevision, string(req.Channels), req.HealthyChannels, req.TotalChannels, req.ReportedAt).Error; err != nil {
			return err
		}
		if previous != "" && previous != req.ConfigRevision {
			summary, err := json.Marshal(map[string]int{"healthyChannels": req.HealthyChannels, "totalChannels": req.TotalChannels})
			if err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO site_config_events(site_id,event_type,config_revision,summary_json) VALUES (?,'channels.changed',?,?::jsonb)`, siteID, req.ConfigRevision, string(summary)).Error
		}
		return nil
	})
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/F25731/zhimeng/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) CreateCodes(c *gin.Context) {
	var req struct {
		Remark         string     `json:"remark"`
		Quantity       int        `json:"quantity"`
		MaxSites       int        `json:"maxSites"`
		ExpiresAt      *time.Time `json:"expiresAt"`
		InitialVersion string     `json:"initialVersion"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	codes, err := h.service.CreateCodes(req.Remark, req.Quantity, req.MaxSites, req.ExpiresAt, req.InitialVersion)
	if err != nil {
		bad(c, err.Error())
		return
	}
	h.audit(c, "codes.create", "provision_code", "", gin.H{"quantity": req.Quantity, "remark": req.Remark})
	httpx.OK(c, gin.H{"codes": codes})
}
func (h *Handler) ListCodes(c *gin.Context) {
	rows, total, err := h.service.ListCodes(page(c), size(c), c.Query("status"), c.Query("query"))
	respondList(c, rows, total, err)
}
func (h *Handler) RevokeCode(c *gin.Context) {
	if err := h.service.RevokeCode(c.Param("id")); err != nil {
		server(c, err)
		return
	}
	h.audit(c, "codes.revoke", "provision_code", c.Param("id"), nil)
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) DeleteCode(c *gin.Context) {
	if err := h.service.DeleteCode(c.Param("id")); err != nil {
		server(c, err)
		return
	}
	h.audit(c, "codes.delete", "provision_code", c.Param("id"), nil)
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) ExportCodes(c *gin.Context) {
	rows, _, err := h.service.ListCodes(1, 1000, "", "")
	if err != nil {
		server(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=provision-codes.csv")
	w := bufio.NewWriter(c.Writer)
	defer w.Flush()
	_, _ = w.WriteString("prefix,remark,status,max_sites,used_sites,expires_at,created_at\n")
	for _, r := range rows {
		_, _ = w.WriteString(fmt.Sprintf("%s,%q,%s,%d,%d,%v,%s\n", r.Prefix, r.Remark, r.Status, r.MaxSites, r.UsedSites, r.ExpiresAt, r.CreatedAt.Format(time.RFC3339)))
	}
}

func (h *Handler) VerifyCode(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	token, session, resume, err := h.service.VerifyCode(req.Code, c.ClientIP())
	if err != nil {
		bad(c, err.Error())
		return
	}
	if resume != nil {
		httpx.OK(c, gin.H{"provisionToken": token, "expiresAt": session.ExpiresAt, "resumeJob": resume})
		return
	}
	response := gin.H{"provisionToken": token, "expiresAt": session.ExpiresAt}
	if session.Reservation != nil {
		response["resumeReservation"] = session.Reservation
	}
	httpx.OK(c, response)
}
func (h *Handler) CheckDomain(c *gin.Context) {
	var req struct {
		Prefix string `json:"prefix"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	reservation, err := h.service.ReserveDomain(provisionToken(c), req.Prefix)
	if err != nil {
		bad(c, err.Error())
		return
	}
	httpx.OK(c, gin.H{"available": true, "domain": reservation.Domain, "reservationId": reservation.ID, "expiresAt": reservation.ExpiresAt})
}
func (h *Handler) UploadLogo(c *gin.Context) {
	if _, err := h.service.sessionID(provisionToken(c)); err != nil {
		httpx.Error(c, http.StatusUnauthorized, 40120, "provision session required")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		bad(c, "logo file is required")
		return
	}
	if file.Size > 1024*1024 {
		bad(c, "logo must be smaller than 1MB")
		return
	}
	extension := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
	if !allowed[extension] {
		bad(c, "unsupported logo format")
		return
	}
	opened, err := file.Open()
	if err != nil {
		bad(c, "invalid logo file")
		return
	}
	header := make([]byte, 512)
	n, _ := io.ReadFull(opened, header)
	_ = opened.Close()
	mime := http.DetectContentType(header[:n])
	allowedMime := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}
	if !allowedMime[mime] {
		bad(c, "unsupported logo content")
		return
	}
	if err := os.MkdirAll("uploads", 0750); err != nil {
		server(c, err)
		return
	}
	filename := uuid.NewString() + extension
	path := filepath.Join("uploads", filename)
	if err := c.SaveUploadedFile(file, path); err != nil {
		server(c, err)
		return
	}
	base := strings.TrimRight(h.service.cfg.PublicBaseURL, "/")
	url := "/uploads/" + filename
	if base != "" {
		url = base + url
	}
	httpx.OK(c, gin.H{"url": url})
}
func (h *Handler) ImportLogo(c *gin.Context) {
	if _, err := h.service.sessionID(provisionToken(c)); err != nil {
		httpx.Error(c, http.StatusUnauthorized, 40120, "provision session required")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "图片地址不能为空")
		return
	}
	storedURL, err := h.service.ImportSiteImage(c.Request.Context(), req.URL)
	if err != nil {
		bad(c, err.Error())
		return
	}
	httpx.OK(c, gin.H{"url": storedURL})
}
func (h *Handler) CreateJob(c *gin.Context) {
	var req struct {
		ReservationID string               `json:"reservationId"`
		Prefix        string               `json:"prefix"`
		Name          string               `json:"name"`
		LogoURL       string               `json:"logoUrl"`
		SiteProfile   siteBootstrapProfile `json:"siteProfile"`
		AdminUsername string               `json:"adminUsername"`
		AdminPassword string               `json:"adminPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	if req.SiteProfile.LogoURL == "" {
		req.SiteProfile.LogoURL = req.LogoURL
	}
	job, err := h.service.CreateProvisionJob(CreateSiteInput{Token: provisionToken(c), ReservationID: req.ReservationID, Prefix: req.Prefix, Name: req.Name, SiteProfile: req.SiteProfile, AdminUsername: req.AdminUsername, AdminPassword: req.AdminPassword})
	if err != nil {
		bad(c, err.Error())
		return
	}
	httpx.OK(c, gin.H{"job": job})
}
func (h *Handler) GetPublicJob(c *gin.Context) {
	if !h.service.CanAccessPublicJob(provisionToken(c), c.Param("id")) {
		httpx.Error(c, http.StatusUnauthorized, 40120, "provision session required")
		return
	}
	job, err := h.service.GetJob(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	events, err := h.service.JobEvents(job.ID, 0)
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"job": job, "events": events})
}
func (h *Handler) RetryPublicJob(c *gin.Context) {
	if !h.service.CanAccessPublicJob(provisionToken(c), c.Param("id")) {
		httpx.Error(c, http.StatusUnauthorized, 40120, "provision session required")
		return
	}
	if err := h.service.RetryJob(c.Param("id")); err != nil {
		bad(c, err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) PublicEvents(c *gin.Context) {
	if !h.service.CanAccessPublicJob(provisionToken(c), c.Param("id")) {
		httpx.Error(c, http.StatusUnauthorized, 40120, "provision session required")
		return
	}
	h.events(c, c.Param("id"), true)
}

func (h *Handler) Dashboard(c *gin.Context) {
	data, err := h.service.Dashboard()
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, data)
}
func (h *Handler) ListSites(c *gin.Context) {
	rows, total, err := h.service.ListSites(page(c), size(c), c.Query("status"), c.Query("query"))
	respondList(c, rows, total, err)
}
func (h *Handler) SiteDetail(c *gin.Context) {
	row, err := h.service.SiteDetail(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	httpx.OK(c, row)
}
func (h *Handler) SiteMetrics(c *gin.Context) {
	rows, err := h.service.SiteMetrics(c.Param("id"))
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": rows})
}
func (h *Handler) SiteChannels(c *gin.Context) {
	row, err := h.service.SiteChannels(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	httpx.OK(c, row)
}
func (h *Handler) SiteAction(c *gin.Context) {
	var req struct {
		Confirmation string `json:"confirmation"`
		Version      string `json:"version"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			bad(c, "请求参数格式错误")
			return
		}
	}
	if req.Version == "" {
		req.Version = c.Query("version")
	}
	job, err := h.service.CreateSiteJob(c.Param("id"), c.Param("action"), req.Version, req.Confirmation)
	if err != nil {
		bad(c, err.Error())
		return
	}
	h.audit(c, "sites."+c.Param("action"), "site", c.Param("id"), gin.H{"jobId": job.ID, "targetVersion": req.Version})
	httpx.OK(c, gin.H{"job": job})
}
func (h *Handler) ListJobs(c *gin.Context) {
	rows, total, err := h.service.ListJobs(page(c), size(c), c.Query("status"), c.Query("type"), c.Query("siteId"), c.Query("query"))
	respondList(c, rows, total, err)
}
func (h *Handler) GetJob(c *gin.Context) {
	job, err := h.service.GetJob(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	httpx.OK(c, gin.H{"job": job})
}
func (h *Handler) RetryJob(c *gin.Context) {
	if err := h.service.RetryJob(c.Param("id")); err != nil {
		bad(c, err.Error())
		return
	}
	h.audit(c, "jobs.retry", "deployment_job", c.Param("id"), nil)
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) JobEvents(c *gin.Context) { h.events(c, c.Param("id"), false) }
func (h *Handler) ListVersions(c *gin.Context) {
	rows, err := h.service.ListVersions()
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": rows})
}
func (h *Handler) CreateVersion(c *gin.Context) {
	var req struct {
		Version           string `json:"version"`
		Image             string `json:"image"`
		Channel           string `json:"channel"`
		Notes             string `json:"releaseNotes"`
		MigrationVersion  string `json:"migrationVersion"`
		MinUpgradeVersion string `json:"minUpgradeVersion"`
		ForceUpgrade      bool   `json:"forceUpgrade"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		bad(c, "invalid request")
		return
	}
	id, err := h.service.CreateVersion(req.Version, req.Image, req.Channel, req.Notes, req.MigrationVersion, req.MinUpgradeVersion, req.ForceUpgrade)
	if err != nil {
		bad(c, err.Error())
		return
	}
	h.audit(c, "versions.create", "release_version", id, gin.H{"version": req.Version, "channel": req.Channel})
	httpx.OK(c, gin.H{"id": id})
}
func (h *Handler) PublishVersion(c *gin.Context) {
	if err := h.service.PublishVersion(c.Param("id")); err != nil {
		server(c, err)
		return
	}
	h.audit(c, "versions.publish", "release_version", c.Param("id"), nil)
	httpx.OK(c, gin.H{"ok": true})
}
func (h *Handler) ListNodes(c *gin.Context) {
	rows, err := h.service.ListNodes()
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": rows})
}
func (h *Handler) ListAudit(c *gin.Context) {
	rows, total, err := h.service.ListAudit(page(c), size(c), c.Query("action"), c.Query("query"))
	respondList(c, rows, total, err)
}

func (h *Handler) events(c *gin.Context, jobID string, public bool) {
	if _, err := h.service.GetJob(jobID); err != nil {
		notFound(c)
		return
	}
	last, _ := strconv.ParseInt(c.GetHeader("Last-Event-ID"), 10, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	fmt.Fprint(c.Writer, "retry: 2000\n\n")
	flusher.Flush()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := h.service.JobEvents(jobID, last)
		if err != nil {
			return
		}
		for _, event := range events {
			message := event.PublicMessage
			if !public {
				if event.InternalMessage != "" {
					message = event.InternalMessage
				}
			}
			payload, _ := json.Marshal(gin.H{"step": event.Step, "status": event.Status, "progress": event.Progress, "message": message, "createdAt": event.CreatedAt})
			fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.EventType, payload)
			last = event.Sequence
		}
		flusher.Flush()
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func provisionToken(c *gin.Context) string {
	if token := c.GetHeader("X-Provision-Token"); token != "" {
		return token
	}
	return c.Query("token")
}
func page(c *gin.Context) int { v, _ := strconv.Atoi(c.DefaultQuery("page", "1")); return v }
func size(c *gin.Context) int { v, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20")); return v }
func respondList(c *gin.Context, rows interface{}, total int64, err error) {
	if err != nil {
		server(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": rows, "total": total, "page": page(c), "pageSize": size(c)})
}
func bad(c *gin.Context, message string) { httpx.Error(c, http.StatusBadRequest, 40000, message) }
func notFound(c *gin.Context)            { httpx.Error(c, http.StatusNotFound, 40400, "not found") }
func server(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		notFound(c)
		return
	}
	httpx.Error(c, http.StatusInternalServerError, 50000, "request failed")
}

func (h *Handler) audit(c *gin.Context, action, targetType, targetID string, detail any) {
	user, ok := c.Get("admin_user")
	if !ok {
		return
	}
	admin := user.(models.AdminUser)
	encoded, _ := json.Marshal(detail)
	if len(encoded) == 0 || string(encoded) == "null" {
		encoded = []byte("{}")
	}
	_ = h.service.db.Exec(`INSERT INTO audit_logs(admin_user_id,action,target_type,target_id,ip,user_agent,detail_json) VALUES (?,?,?,?,NULLIF(?,'')::inet,?,?::jsonb)`, admin.ID, action, targetType, targetID, c.ClientIP(), c.Request.UserAgent(), string(encoded)).Error
}

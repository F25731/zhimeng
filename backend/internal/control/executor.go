package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/security"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var safeSQLIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}$`)

type siteRuntime struct {
	ID                   string
	ProvisionCodeID      string
	Name                 string
	Domain               string
	LogoURL              string
	Status               string
	CurrentVersion       string
	DesiredVersion       string
	DatabaseName         string
	DatabaseUser         string
	AppContainerName     string
	WorkerContainerName  string
	BootstrapProfileJSON string
}

type siteCredentials struct {
	DatabasePassword string
	EncryptionKey    string
	MaintenanceToken string
	ReportToken      string
	AdminPassword    string
}

type siteTemplateData struct {
	SiteID           string
	Domain           string
	SiteImage        string
	ReporterImage    string
	DockerNetwork    string
	DatabaseNetwork  string
	DatabaseURL      string
	EncryptionKey    string
	MaintenanceToken string
	ControlCenterURL string
	ControlSiteToken string
	Version          string
	RouteEnabled     bool
}

type renderedComposeConfig struct {
	Services map[string]renderedComposeService `json:"services"`
}

type renderedComposeService struct {
	Image         string            `json:"image"`
	ContainerName string            `json:"container_name"`
	Environment   map[string]string `json:"environment"`
}

type siteReadinessResponse struct {
	Data struct {
		Ready            bool `json:"ready"`
		GenerationWorker struct {
			Healthy         bool   `json:"healthy"`
			LastHeartbeatAt string `json:"lastHeartbeatAt"`
			Reason          string `json:"reason"`
		} `json:"generationWorker"`
	} `json:"data"`
}

type commandRunner interface {
	Run(context.Context, string, ...string) (string, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 16*1024 {
		output = output[len(output)-16*1024:]
	}
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

type siteExecutor struct {
	cfg    config.Config
	db     *gorm.DB
	runner commandRunner
	client *http.Client
}

func newSiteExecutor(cfg config.Config, db *gorm.DB) *siteExecutor {
	jar, _ := cookiejar.New(nil)
	return &siteExecutor{
		cfg:    cfg,
		db:     db,
		runner: osCommandRunner{},
		client: &http.Client{Timeout: 15 * time.Second, Jar: jar},
	}
}

func (e *siteExecutor) loadSite(id string) (siteRuntime, error) {
	var site siteRuntime
	err := e.db.Raw(`SELECT id,COALESCE(provision_code_id::text,'') provision_code_id,name,domain,COALESCE(logo_url,'') logo_url,status,COALESCE(current_version,'latest') current_version,COALESCE(desired_version,'latest') desired_version,database_name,database_user,app_container_name,worker_container_name,COALESCE(bootstrap_profile,'{}'::jsonb)::text bootstrap_profile_json FROM sites WHERE id=? AND deleted_at IS NULL`, id).Scan(&site).Error
	if err != nil || site.ID == "" {
		return siteRuntime{}, gorm.ErrRecordNotFound
	}
	return site, nil
}

func (e *siteExecutor) ensureNode(siteID string) error {
	return e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO deployment_nodes(name,status,agent_version,last_heartbeat_at) VALUES (?,'online','local',now()) ON CONFLICT DO NOTHING`, e.cfg.AgentID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE sites SET node_id=(SELECT id FROM deployment_nodes WHERE name=? ORDER BY created_at LIMIT 1),updated_at=now() WHERE id=?`, e.cfg.AgentID, siteID).Error
	})
}

func (e *siteExecutor) ensureCredentials(siteID string, includeBootstrap bool) (siteCredentials, error) {
	values := map[string]string{}
	kinds := []string{"database_password", "encryption_key", "maintenance_token", "report_token"}
	if includeBootstrap {
		kinds = append(kinds, "bootstrap_admin_password")
	}
	for _, kind := range kinds {
		var ciphertext string
		if err := e.db.Raw(`SELECT ciphertext FROM site_secrets WHERE site_id=? AND secret_type=?`, siteID, kind).Scan(&ciphertext).Error; err != nil {
			return siteCredentials{}, err
		}
		if ciphertext == "" {
			var plain string
			var err error
			if kind == "encryption_key" {
				plain, err = randomHex(32)
			} else {
				plain, err = security.RandomToken(36)
			}
			if err != nil {
				return siteCredentials{}, err
			}
			ciphertext, err = security.Seal(plain, e.cfg.MasterEncryptionKey)
			if err != nil {
				return siteCredentials{}, err
			}
			if err := e.db.Exec(`INSERT INTO site_secrets(site_id,secret_type,ciphertext) VALUES (?,?,?) ON CONFLICT(site_id,secret_type) DO NOTHING`, siteID, kind, ciphertext).Error; err != nil {
				return siteCredentials{}, err
			}
			if err := e.db.Raw(`SELECT ciphertext FROM site_secrets WHERE site_id=? AND secret_type=?`, siteID, kind).Scan(&ciphertext).Error; err != nil {
				return siteCredentials{}, err
			}
		}
		plain, err := security.Open(ciphertext, e.cfg.MasterEncryptionKey)
		if err != nil {
			return siteCredentials{}, err
		}
		values[kind] = plain
	}
	return siteCredentials{
		DatabasePassword: values["database_password"], EncryptionKey: values["encryption_key"],
		MaintenanceToken: values["maintenance_token"], ReportToken: values["report_token"],
		AdminPassword: values["bootstrap_admin_password"],
	}, nil
}

func (e *siteExecutor) ensureDatabase(ctx context.Context, site siteRuntime, password string) error {
	if !safeSQLIdentifier.MatchString(site.DatabaseName) || !safeSQLIdentifier.MatchString(site.DatabaseUser) {
		return errors.New("unsafe generated database identifier")
	}
	if strings.TrimSpace(e.cfg.PostgresAdminURL) == "" {
		return errors.New("POSTGRES_ADMIN_URL is required")
	}
	adminDB, err := gorm.Open(postgres.Open(e.cfg.PostgresAdminURL), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := adminDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}

	var roleExists bool
	if err := adminDB.Raw(`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=?)`, site.DatabaseUser).Scan(&roleExists).Error; err != nil {
		return err
	}
	quotedUser := quoteIdentifier(site.DatabaseUser)
	if !roleExists {
		if err := adminDB.Exec(`CREATE ROLE ` + quotedUser + ` LOGIN PASSWORD ` + quoteLiteral(password)).Error; err != nil {
			return err
		}
	} else if err := adminDB.Exec(`ALTER ROLE ` + quotedUser + ` WITH LOGIN PASSWORD ` + quoteLiteral(password)).Error; err != nil {
		return err
	}
	var databaseExists bool
	if err := adminDB.Raw(`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=?)`, site.DatabaseName).Scan(&databaseExists).Error; err != nil {
		return err
	}
	if !databaseExists {
		if err := adminDB.Exec(`CREATE DATABASE ` + quoteIdentifier(site.DatabaseName) + ` OWNER ` + quotedUser).Error; err != nil {
			return err
		}
	}
	return adminDB.Exec(`GRANT ALL PRIVILEGES ON DATABASE ` + quoteIdentifier(site.DatabaseName) + ` TO ` + quotedUser).Error
}

func (e *siteExecutor) destroyRuntime(ctx context.Context, site siteRuntime, dir string) error {
	if _, err := uuid.Parse(site.ID); err != nil {
		return errors.New("unsafe site identifier")
	}
	composePath := filepath.Join(dir, "compose.yml")
	if _, err := os.Stat(composePath); err == nil {
		if err := e.compose(ctx, dir, "down", "--volumes", "--remove-orphans"); err != nil {
			return fmt.Errorf("remove site compose stack: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{"site-" + site.ID + "-app", "site-" + site.ID + "-worker", "site-" + site.ID + "-reporter"} {
		if err := e.removeDockerObject(ctx, "container", name); err != nil {
			return err
		}
	}
	return e.removeDockerObject(ctx, "volume", "site-"+site.ID+"-data")
}

func (e *siteExecutor) removeDockerObject(ctx context.Context, kind, name string) error {
	output, err := e.runner.Run(ctx, "docker", kind, "inspect", name)
	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "no such") || strings.Contains(lower, "not found") {
			return nil
		}
		return fmt.Errorf("inspect docker %s %s: %w", kind, name, err)
	}
	if _, err := e.runner.Run(ctx, "docker", kind, "rm", "-f", name); err != nil {
		return fmt.Errorf("remove docker %s %s: %w", kind, name, err)
	}
	return nil
}

func (e *siteExecutor) destroyDatabase(ctx context.Context, site siteRuntime) error {
	if !safeSQLIdentifier.MatchString(site.DatabaseName) || !safeSQLIdentifier.MatchString(site.DatabaseUser) {
		return errors.New("unsafe generated database identifier")
	}
	if strings.TrimSpace(e.cfg.PostgresAdminURL) == "" {
		return errors.New("POSTGRES_ADMIN_URL is required")
	}
	adminDB, err := gorm.Open(postgres.Open(e.cfg.PostgresAdminURL), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := adminDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}
	db := adminDB.WithContext(ctx)
	if err := db.Exec(`DROP DATABASE IF EXISTS ` + quoteIdentifier(site.DatabaseName) + ` WITH (FORCE)`).Error; err != nil {
		return fmt.Errorf("drop site database: %w", err)
	}
	if err := db.Exec(`DROP ROLE IF EXISTS ` + quoteIdentifier(site.DatabaseUser)).Error; err != nil {
		return fmt.Errorf("drop site database role: %w", err)
	}
	return nil
}

func (e *siteExecutor) removeSiteDirectory(siteID string) error {
	if _, err := uuid.Parse(siteID); err != nil {
		return errors.New("unsafe site identifier")
	}
	base, err := filepath.Abs(e.cfg.SiteBaseDir)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Join(base, siteID))
	if err != nil {
		return err
	}
	if target != filepath.Join(base, siteID) {
		return errors.New("site directory escaped configured base directory")
	}
	return os.RemoveAll(target)
}

func (e *siteExecutor) resolveImage(version string) (string, error) {
	var image string
	if version != "" && version != "latest" {
		if err := e.db.Raw(`SELECT image FROM release_versions WHERE version=? AND status='published'`, version).Scan(&image).Error; err != nil {
			return "", err
		}
		if image == "" {
			return "", fmt.Errorf("published release %s not found", version)
		}
	}
	if image == "" {
		image = strings.TrimSpace(e.cfg.SiteImageDefault)
	}
	if image == "" {
		return "", errors.New("SITE_IMAGE_DEFAULT is required")
	}
	return image, nil
}

func (e *siteExecutor) renderSite(site siteRuntime, credentials siteCredentials, image, version string, routeEnabled bool) (string, error) {
	databaseURL, err := databaseURL(e.cfg.PostgresAdminURL, site.DatabaseUser, credentials.DatabasePassword, site.DatabaseName)
	if err != nil {
		return "", err
	}
	data := siteTemplateData{
		SiteID: site.ID, Domain: site.Domain, SiteImage: image, ReporterImage: e.cfg.ReporterImage,
		DockerNetwork: e.cfg.DockerNetwork, DatabaseNetwork: e.cfg.DatabaseNetwork,
		DatabaseURL: databaseURL, EncryptionKey: credentials.EncryptionKey,
		MaintenanceToken: credentials.MaintenanceToken, ControlCenterURL: e.cfg.PublicBaseURL,
		ControlSiteToken: credentials.ReportToken, Version: version, RouteEnabled: routeEnabled,
	}
	dir := filepath.Join(e.cfg.SiteBaseDir, site.ID)
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0750); err != nil {
		return "", err
	}
	if err := renderFile(filepath.Join(e.cfg.SiteTemplateDir, "site-env.tmpl"), filepath.Join(dir, ".env"), data, 0600); err != nil {
		return "", err
	}
	if err := renderFile(filepath.Join(e.cfg.SiteTemplateDir, "site-compose.yml.tmpl"), filepath.Join(dir, "compose.yml"), data, 0640); err != nil {
		return "", err
	}
	return dir, nil
}

func (e *siteExecutor) compose(ctx context.Context, dir string, args ...string) error {
	base := e.composeArgs(dir)
	_, err := e.runner.Run(ctx, "docker", append(base, args...)...)
	return err
}

func (e *siteExecutor) composeArgs(dir string) []string {
	return []string{"compose", "--project-directory", dir, "-f", filepath.Join(dir, "compose.yml")}
}

func (e *siteExecutor) loadComposeConfig(ctx context.Context, dir string) (renderedComposeConfig, error) {
	output, err := e.runner.Run(ctx, "docker", append(e.composeArgs(dir), "config", "--format", "json")...)
	if err != nil {
		return renderedComposeConfig{}, fmt.Errorf("validate generated compose file: %w", err)
	}
	var rendered renderedComposeConfig
	if err := json.Unmarshal([]byte(output), &rendered); err != nil {
		return renderedComposeConfig{}, fmt.Errorf("parse generated compose configuration: %w", err)
	}
	if err := validateComposeContract(rendered); err != nil {
		return renderedComposeConfig{}, err
	}
	return rendered, nil
}

func validateComposeContract(rendered renderedComposeConfig) error {
	app, appOK := rendered.Services["app"]
	worker, workerOK := rendered.Services["worker"]
	reporter, reporterOK := rendered.Services["reporter"]
	if !appOK || !workerOK || !reporterOK {
		return errors.New("generated compose must contain app, worker and reporter services")
	}
	if strings.TrimSpace(app.Image) == "" || app.Image != worker.Image {
		return errors.New("app and worker must use the same non-empty site image")
	}
	if strings.TrimSpace(reporter.Image) == "" {
		return errors.New("reporter image is required")
	}
	if app.Environment["VOZEB_PRO_DATABASE_PROVIDER"] != "postgres" || strings.TrimSpace(app.Environment["DATABASE_URL"]) == "" {
		return errors.New("app must use its provisioned PostgreSQL database")
	}
	if err := validateSiteDatabaseContract(app, reporter); err != nil {
		return err
	}
	if app.Environment["VOZEB_PRO_INTERNAL_ORIGIN"] != "http://127.0.0.1:3000" {
		return errors.New("app internal origin must address the app container itself")
	}
	if worker.Environment["VOZEB_PRO_WORKER_API_ORIGIN"] != "http://app:3000" {
		return errors.New("worker API origin must use the Docker app service address")
	}
	if worker.Environment["DATABASE_URL"] != "" || worker.Environment["VOZEB_PRO_DATABASE_PROVIDER"] != "" {
		return errors.New("worker must call the app API and must not receive database credentials")
	}
	if len(strings.TrimSpace(worker.Environment["VOZEB_PRO_MAINTENANCE_TOKEN"])) < 32 || worker.Environment["VOZEB_PRO_MAINTENANCE_TOKEN"] != app.Environment["VOZEB_PRO_MAINTENANCE_TOKEN"] {
		return errors.New("app and worker must share a valid maintenance token")
	}
	if strings.TrimSpace(reporter.Environment["DATABASE_URL"]) == "" || reporter.Environment["SITE_DATABASE_TABLE_PREFIX"] != "vozeb_pro_" {
		return errors.New("reporter database contract is incomplete")
	}
	return nil
}

func validateSiteDatabaseContract(app, reporter renderedComposeService) error {
	databaseURL := strings.TrimSpace(app.Environment["DATABASE_URL"])
	if databaseURL == "" || databaseURL != strings.TrimSpace(reporter.Environment["DATABASE_URL"]) {
		return errors.New("app and reporter must use the same provisioned database")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.User == nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return errors.New("provisioned database URL is invalid")
	}
	siteID := strings.TrimSuffix(strings.TrimPrefix(app.ContainerName, "site-"), "-app")
	if _, err := uuid.Parse(siteID); err != nil {
		return errors.New("app container name does not contain a valid site identifier")
	}
	suffix := strings.ReplaceAll(siteID, "-", "")
	if strings.TrimPrefix(parsed.Path, "/") != "site_"+suffix || parsed.User.Username() != "user_"+suffix {
		return errors.New("generated compose points outside the current site's isolated database")
	}
	return nil
}

func (e *siteExecutor) prepareImages(ctx context.Context, dir string) error {
	rendered, err := e.loadComposeConfig(ctx, dir)
	if err != nil {
		return err
	}
	images := map[string]struct{}{}
	for _, service := range rendered.Services {
		if image := strings.TrimSpace(service.Image); image != "" {
			images[image] = struct{}{}
		}
	}
	for image := range images {
		if _, inspectErr := e.runner.Run(ctx, "docker", "image", "inspect", image); inspectErr == nil {
			continue
		}
		if _, pullErr := e.runner.Run(ctx, "docker", "pull", image); pullErr != nil {
			return fmt.Errorf("prepare required image %q: %w", image, pullErr)
		}
	}
	return nil
}

func (e *siteExecutor) waitForHealth(ctx context.Context, site siteRuntime, ready bool) error {
	path := "/api/health/live"
	if ready {
		path = "/api/health/ready"
	}
	endpoint := "http://" + site.AppContainerName + ":3000" + path
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		resp, err := e.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("site health check timed out: %s", path)
}

func (e *siteExecutor) waitForWorker(ctx context.Context, site siteRuntime) error {
	timeout := time.Duration(e.cfg.WorkerReadyTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	endpoint := "http://" + site.AppContainerName + ":3000/api/health/ready"
	lastReason := "heartbeat_missing"
	for time.Now().Before(deadline) {
		readiness, err := e.readReadiness(ctx, endpoint)
		if err == nil {
			if readiness.Data.GenerationWorker.Healthy {
				return nil
			}
			if readiness.Data.GenerationWorker.Reason != "" {
				lastReason = readiness.Data.GenerationWorker.Reason
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("generation worker did not become ready: %s", lastReason)
}

func (e *siteExecutor) readReadiness(ctx context.Context, endpoint string) (siteReadinessResponse, error) {
	var readiness siteReadinessResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return readiness, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return readiness, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&readiness); err != nil {
		return readiness, err
	}
	return readiness, nil
}

func (e *siteExecutor) waitForRoute(ctx context.Context, site siteRuntime, public bool) error {
	timeout := time.Duration(e.cfg.RouteProbeTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := e.probeRoute(ctx, site, public); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = errors.New("route did not become ready")
	}
	return lastErr
}

func (e *siteExecutor) probeRoute(ctx context.Context, site siteRuntime, public bool) error {
	base := strings.TrimRight(e.cfg.SiteRouterURL, "/")
	if public {
		base = "https://" + site.Domain
	}
	endpoint := base + "/api/health/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if !public {
		req.Host = site.Domain
	}
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("route probe returned %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Control-Site-ID") != site.ID {
		return errors.New("route probe reached an unexpected site")
	}
	return nil
}

func (e *siteExecutor) bootstrap(ctx context.Context, site siteRuntime, adminUsername, adminPassword, maintenanceToken string) error {
	baseURL := "http://" + site.AppContainerName + ":3000"
	status := struct {
		Install struct {
			Ready              bool `json:"ready"`
			FirstAdminRequired bool `json:"firstAdminRequired"`
			Database           struct {
				SchemaReady bool `json:"schemaReady"`
			} `json:"database"`
		} `json:"install"`
	}{}
	if err := e.doJSON(ctx, http.MethodPost, baseURL+"/api/install/initialize", nil, nil); err != nil {
		return fmt.Errorf("initialize site database: %w", err)
	}
	if err := e.doJSON(ctx, http.MethodGet, baseURL+"/api/install/status", nil, &status); err != nil {
		return err
	}
	if !status.Install.Ready && !status.Install.Database.SchemaReady {
		return errors.New("site database initialization did not produce a ready schema")
	}
	if status.Install.FirstAdminRequired {
		payload := map[string]string{"username": adminUsername, "displayName": adminUsername, "password": adminPassword}
		if err := e.doJSON(ctx, http.MethodPost, baseURL+"/api/auth/register", payload, nil); err != nil {
			return fmt.Errorf("create site administrator: %w", err)
		}
	} else {
		payload := map[string]string{"username": adminUsername, "password": adminPassword}
		if err := e.doJSON(ctx, http.MethodPost, baseURL+"/api/auth/login", payload, nil); err != nil {
			return fmt.Errorf("authenticate site administrator: %w", err)
		}
	}
	var rawProfile siteBootstrapProfile
	if err := json.Unmarshal([]byte(site.BootstrapProfileJSON), &rawProfile); err != nil {
		return fmt.Errorf("decode site bootstrap profile: %w", err)
	}
	profile, err := normalizeSiteBootstrapProfile(rawProfile)
	if err != nil {
		return fmt.Errorf("validate site bootstrap profile: %w", err)
	}
	updated := struct {
		Settings struct {
			Site siteBootstrapProfile `json:"site"`
		} `json:"settings"`
	}{}
	if err := e.doMaintenanceJSON(ctx, http.MethodPost, baseURL+"/api/maintenance/site-profile", maintenanceToken, map[string]any{"site": profile.settingsPayload()}, &updated); err != nil {
		return err
	}
	return verifyAppliedSiteProfile(profile, updated.Settings.Site)
}

func verifyAppliedSiteProfile(expected, actual siteBootstrapProfile) error {
	fields := map[string][2]string{
		"title": {expected.Title, actual.Title}, "logoUrl": {expected.LogoURL, actual.LogoURL},
		"iconUrl": {expected.IconURL, actual.IconURL}, "seoTitle": {expected.SEOTitle, actual.SEOTitle},
		"seoDescription": {expected.SEODescription, actual.SEODescription}, "seoKeywords": {expected.SEOKeywords, actual.SEOKeywords},
		"footerCopyright": {expected.FooterCopyright, actual.FooterCopyright}, "termsUrl": {expected.TermsURL, actual.TermsURL},
		"privacyUrl": {expected.PrivacyURL, actual.PrivacyURL},
	}
	for name, values := range fields {
		if values[0] != values[1] {
			return fmt.Errorf("site setting %s was not applied exactly", name)
		}
	}
	encoded, _ := json.Marshal(actual)
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"vozeb pro", "csyqlz@gmail.com", "qm.qq.com/q/9mvltxurd6", "www.vozeb.com"} {
		if strings.Contains(lower, forbidden) {
			return fmt.Errorf("site settings still contain forbidden upstream branding: %s", forbidden)
		}
	}
	return nil
}

func (e *siteExecutor) doJSON(ctx context.Context, method, endpoint string, payload, target any) error {
	return e.doJSONWithToken(ctx, method, endpoint, "", payload, target)
}

func (e *siteExecutor) doMaintenanceJSON(ctx context.Context, method, endpoint, token string, payload, target any) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("site maintenance token is missing")
	}
	return e.doJSONWithToken(ctx, method, endpoint, token, payload, target)
}

func (e *siteExecutor) doJSONWithToken(ctx context.Context, method, endpoint, token string, payload, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if target != nil && len(responseBody) > 0 {
		return json.Unmarshal(responseBody, target)
	}
	return nil
}

func (e *siteExecutor) backup(ctx context.Context, site siteRuntime, credentials siteCredentials, jobID string) (string, error) {
	dir := filepath.Join(e.cfg.SiteBaseDir, site.ID, "backups")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405Z")+"-"+jobID+".dump")
	dsn, err := databaseURL(e.cfg.PostgresAdminURL, site.DatabaseUser, credentials.DatabasePassword, site.DatabaseName)
	if err != nil {
		return "", err
	}
	if _, err := e.runner.Run(ctx, "pg_dump", "--format=custom", "--no-owner", "--file", path, dsn); err != nil {
		return "", err
	}
	return path, nil
}

func renderFile(templatePath, targetPath string, data any, mode os.FileMode) error {
	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := tpl.Execute(&output, data); err != nil {
		return err
	}
	temporary := targetPath + ".tmp"
	if err := os.WriteFile(temporary, output.Bytes(), mode); err != nil {
		return err
	}
	if err := os.Chmod(temporary, mode); err != nil {
		return err
	}
	return os.Rename(temporary, targetPath)
}

func databaseURL(adminURL, username, password, databaseName string) (string, error) {
	parsed, err := url.Parse(adminURL)
	if err != nil {
		return "", err
	}
	parsed.User = url.UserPassword(username, password)
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
func quoteLiteral(value string) string    { return `'` + strings.ReplaceAll(value, `'`, `''`) + `'` }

func randomHex(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

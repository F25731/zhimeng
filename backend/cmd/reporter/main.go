package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type reporterConfig struct {
	databaseURL string
	controlURL  string
	siteID      string
	token       string
	appURL      string
	version     string
	tablePrefix string
	interval    time.Duration
}

type reporter struct {
	cfg    reporterConfig
	db     *sql.DB
	client *http.Client
	logger *slog.Logger
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("load reporter config failed", "error", err)
		os.Exit(1)
	}
	db, err := sql.Open("pgx", cfg.databaseURL)
	if err != nil {
		slog.Error("open site database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	r := &reporter{
		cfg:    cfg,
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
		logger: slog.Default(),
	}
	if err := r.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("site reporter stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (reporterConfig, error) {
	seconds, _ := strconv.Atoi(os.Getenv("CONTROL_REPORT_INTERVAL_SECONDS"))
	if seconds < 15 {
		seconds = 60
	}
	cfg := reporterConfig{
		databaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		controlURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("CONTROL_CENTER_URL")), "/"),
		siteID:      strings.TrimSpace(os.Getenv("CONTROL_SITE_ID")),
		token:       strings.TrimSpace(os.Getenv("CONTROL_SITE_TOKEN")),
		appURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("REPORT_APP_URL")), "/"),
		version:     strings.TrimSpace(os.Getenv("SITE_VERSION")),
		tablePrefix: strings.TrimSpace(os.Getenv("SITE_DATABASE_TABLE_PREFIX")),
		interval:    time.Duration(seconds) * time.Second,
	}
	if cfg.tablePrefix == "" {
		cfg.tablePrefix = "vozeb_pro_"
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(cfg.tablePrefix) {
		return reporterConfig{}, errors.New("SITE_DATABASE_TABLE_PREFIX must be a safe PostgreSQL identifier prefix")
	}
	if cfg.databaseURL == "" || cfg.controlURL == "" || cfg.siteID == "" || cfg.token == "" {
		return reporterConfig{}, errors.New("DATABASE_URL, CONTROL_CENTER_URL, CONTROL_SITE_ID and CONTROL_SITE_TOKEN are required")
	}
	if cfg.appURL == "" {
		cfg.appURL = "http://app:3000"
	}
	return cfg, nil
}

func (r *reporter) run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.interval)
	defer ticker.Stop()
	for {
		if err := r.report(ctx); err != nil {
			r.logger.Warn("site report cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *reporter) report(ctx context.Context) error {
	now := time.Now().UTC()
	databaseStatus := "healthy"
	if err := r.db.PingContext(ctx); err != nil {
		databaseStatus = "unhealthy"
	}
	appStatus := r.probeApp(ctx)
	workerStatus, runningTasks := r.workerStatus(ctx, now)

	heartbeat := map[string]any{
		"version": r.cfg.version, "appStatus": appStatus, "workerStatus": workerStatus,
		"databaseStatus": databaseStatus, "runningTasks": runningTasks, "reportedAt": now,
	}
	if err := r.send(ctx, "/api/internal/sites/heartbeat", heartbeat); err != nil {
		return err
	}
	if databaseStatus != "healthy" {
		return nil
	}
	schemaReady, err := r.schemaReady(ctx)
	if err != nil {
		return err
	}
	if !schemaReady {
		return nil
	}

	metrics, err := r.metrics(ctx, now)
	if err != nil {
		return err
	}
	if err := r.send(ctx, "/api/internal/sites/metrics", metrics); err != nil {
		return err
	}
	channels, err := r.channels(ctx, now)
	if err != nil {
		return err
	}
	return r.send(ctx, "/api/internal/sites/channels", channels)
}

func (r *reporter) probeApp(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.appURL+"/api/health/live", nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return "unhealthy"
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "unhealthy"
	}
	return "healthy"
}

func (r *reporter) workerStatus(ctx context.Context, now time.Time) (string, int64) {
	var lastSeen sql.NullTime
	_ = r.db.QueryRowContext(ctx, `SELECT max(last_seen_at) FROM `+r.table("generation_worker_heartbeats")).Scan(&lastSeen)
	status := "unhealthy"
	if lastSeen.Valid && now.Sub(lastSeen.Time) < 90*time.Second {
		status = "healthy"
	}
	var running int64
	_ = r.db.QueryRowContext(ctx, `SELECT count(*) FROM `+r.table("generation_tasks")+` WHERE status IN ('pending','running')`).Scan(&running)
	return status, running
}

func (r *reporter) schemaReady(ctx context.Context) (bool, error) {
	var relation sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+r.cfg.tablePrefix+"users").Scan(&relation)
	return relation.Valid && relation.String != "", err
}

func (r *reporter) metrics(ctx context.Context, now time.Time) (map[string]any, error) {
	var usersTotal, usersActive, callsToday, calls7d, callsLifetime, success7d, failed7d int64
	users := r.table("users")
	tasks := r.table("generation_tasks")
	query := fmt.Sprintf(`SELECT
		(SELECT count(*) FROM %s),
		(SELECT count(*) FROM %s WHERE last_login_at >= now() - interval '7 days'),
		(SELECT count(*) FROM %s WHERE created_at >= current_date),
		(SELECT count(*) FROM %s WHERE created_at >= now() - interval '7 days'),
		(SELECT count(*) FROM %s),
		(SELECT count(*) FROM %s WHERE created_at >= now() - interval '7 days' AND status = 'success'),
		(SELECT count(*) FROM %s WHERE created_at >= now() - interval '7 days' AND status = 'error')`, users, users, tasks, tasks, tasks, tasks, tasks)
	if err := r.db.QueryRowContext(ctx, query).Scan(&usersTotal, &usersActive, &callsToday, &calls7d, &callsLifetime, &success7d, &failed7d); err != nil {
		return nil, err
	}
	distribution := map[string]int64{}
	rows, err := r.db.QueryContext(ctx, `SELECT task_type, count(*) FROM `+tasks+` WHERE created_at >= now() - interval '7 days' GROUP BY task_type`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var kind string
			var count int64
			if rows.Scan(&kind, &count) == nil {
				distribution[kind] = count
			}
		}
	}
	totalFinished := success7d + failed7d
	successRate := float64(0)
	if totalFinished > 0 {
		successRate = float64(success7d) * 100 / float64(totalFinished)
	}
	return map[string]any{
		"usersTotal": usersTotal, "usersActive": usersActive, "callsToday": callsToday,
		"calls7d": calls7d, "callsLifetime": callsLifetime, "success7d": success7d,
		"failed7d": failed7d, "activeUsers7d": usersActive, "successRate": successRate,
		"modelDistribution": distribution, "reportedAt": now,
	}, nil
}

func (r *reporter) channels(ctx context.Context, now time.Time) (map[string]any, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,base_url,api_format,enabled,models,health_results,updated_at FROM `+r.table("system_model_channels")+` ORDER BY sort_order,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	healthy := 0
	var revision time.Time
	for rows.Next() {
		var id, name, baseURL, apiFormat string
		var enabled bool
		var models, health json.RawMessage
		var updatedAt time.Time
		if err := rows.Scan(&id, &name, &baseURL, &apiFormat, &enabled, &models, &health, &updatedAt); err != nil {
			return nil, err
		}
		if updatedAt.After(revision) {
			revision = updatedAt
		}
		isHealthy := enabled && !bytes.Contains(bytes.ToLower(health), []byte(`"healthy":false`))
		if isHealthy {
			healthy++
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "baseUrl": baseURL, "apiFormat": apiFormat,
			"enabled": enabled, "models": models, "health": health, "healthy": isHealthy,
		})
	}
	return map[string]any{
		"configRevision": revision.UTC().Format(time.RFC3339Nano), "channels": items,
		"healthyChannels": healthy, "totalChannels": len(items), "reportedAt": now,
	}, rows.Err()
}

func (r *reporter) table(name string) string {
	return `"` + r.cfg.tablePrefix + name + `"`
}

func (r *reporter) send(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := hex.EncodeToString(nonceBytes)
	bodyHash := sha256.Sum256(body)
	signingString := strings.Join([]string{r.cfg.siteID, timestamp, nonce, http.MethodPost, path, hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(r.cfg.token))
	_, _ = mac.Write([]byte(signingString))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.controlURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Site-Id", r.cfg.siteID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("control endpoint %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

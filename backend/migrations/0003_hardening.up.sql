CREATE TABLE IF NOT EXISTS report_nonces (
    site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    nonce varchar(128) NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, nonce)
);
CREATE INDEX IF NOT EXISTS idx_report_nonces_expires_at ON report_nonces(expires_at);

CREATE TABLE IF NOT EXISTS admin_login_attempts (
    id bigserial PRIMARY KEY,
    username varchar(64) NOT NULL,
    ip inet,
    succeeded boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_login_attempts_lookup
    ON admin_login_attempts(username, created_at DESC);

CREATE TABLE IF NOT EXISTS site_daily_metrics (
    site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    metric_date date NOT NULL,
    users_total bigint NOT NULL DEFAULT 0,
    users_active bigint NOT NULL DEFAULT 0,
    calls_total bigint NOT NULL DEFAULT 0,
    calls_success bigint NOT NULL DEFAULT 0,
    calls_failed bigint NOT NULL DEFAULT 0,
    model_distribution_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, metric_date)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_active_site_operation
    ON deployment_jobs(site_id)
    WHERE status IN ('pending', 'running') AND job_type IN ('start','stop','restart','freeze','unfreeze','backup','upgrade');

ALTER TABLE deployment_nodes ADD COLUMN IF NOT EXISTS docker_version varchar(64);
ALTER TABLE deployment_nodes ADD COLUMN IF NOT EXISTS last_error text;
WITH ranked AS (
    SELECT id, first_value(id) OVER (PARTITION BY name ORDER BY created_at, id) AS keep_id,
           row_number() OVER (PARTITION BY name ORDER BY created_at, id) AS row_number
    FROM deployment_nodes
), rewired AS (
    UPDATE sites s SET node_id = r.keep_id
    FROM ranked r WHERE s.node_id = r.id AND r.row_number > 1
)
DELETE FROM deployment_nodes d USING ranked r WHERE d.id = r.id AND r.row_number > 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_nodes_name_unique ON deployment_nodes(name);
ALTER TABLE sites ADD COLUMN IF NOT EXISTS last_error_code varchar(128);
ALTER TABLE sites ADD COLUMN IF NOT EXISTS last_error_message text;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS activated_at timestamptz;

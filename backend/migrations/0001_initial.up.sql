CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS admin_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username varchar(64) NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role varchar(32) NOT NULL DEFAULT 'super_admin',
    status varchar(32) NOT NULL DEFAULT 'active',
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id uuid NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    ip inet,
    user_agent text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS provision_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_prefix varchar(32) NOT NULL,
    code_hash text NOT NULL UNIQUE,
    remark text,
    status varchar(32) NOT NULL DEFAULT 'unused',
    max_sites integer NOT NULL DEFAULT 1,
    used_sites integer NOT NULL DEFAULT 0,
    initial_version varchar(64),
    expires_at timestamptz,
    reserved_at timestamptz,
    bound_site_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_provision_codes_status ON provision_codes(status);
CREATE INDEX IF NOT EXISTS idx_provision_codes_expires_at ON provision_codes(expires_at);

CREATE TABLE IF NOT EXISTS provision_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provision_code_id uuid NOT NULL REFERENCES provision_codes(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    status varchar(32) NOT NULL DEFAULT 'active',
    expires_at timestamptz NOT NULL,
    created_ip inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS domain_reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    prefix varchar(64) NOT NULL,
    full_domain varchar(255) NOT NULL UNIQUE,
    provision_session_id uuid NOT NULL REFERENCES provision_sessions(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_domain_reservations_expires_at ON domain_reservations(expires_at);

CREATE TABLE IF NOT EXISTS deployment_nodes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(128) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'offline',
    public_ip inet,
    agent_token_ciphertext text,
    agent_version varchar(64),
    cpu_total numeric,
    memory_total_mb integer,
    disk_total_gb integer,
    site_count integer NOT NULL DEFAULT 0,
    last_heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provision_code_id uuid REFERENCES provision_codes(id),
    node_id uuid REFERENCES deployment_nodes(id),
    remark text,
    name varchar(128) NOT NULL,
    slug varchar(64) NOT NULL UNIQUE,
    domain varchar(255) NOT NULL UNIQUE,
    logo_url text,
    status varchar(32) NOT NULL DEFAULT 'pending',
    current_version varchar(64),
    desired_version varchar(64),
    database_name varchar(128),
    database_user varchar(128),
    storage_prefix varchar(255),
    app_container_name varchar(255),
    worker_container_name varchar(255),
    last_heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_sites_status ON sites(status);
CREATE INDEX IF NOT EXISTS idx_sites_node_id ON sites(node_id);

CREATE TABLE IF NOT EXISTS site_secrets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    secret_type varchar(64) NOT NULL,
    ciphertext text NOT NULL,
    key_version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(site_id, secret_type)
);

CREATE TABLE IF NOT EXISTS deployment_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id uuid REFERENCES sites(id),
    job_type varchar(32) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'pending',
    current_step varchar(64),
    progress integer NOT NULL DEFAULT 0,
    attempt integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    retryable boolean NOT NULL DEFAULT true,
    error_code varchar(128),
    error_message text,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    result_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    worker_id varchar(128),
    lease_until timestamptz,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_jobs_status_created_at ON deployment_jobs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_deployment_jobs_site_id_created_at ON deployment_jobs(site_id, created_at);
CREATE INDEX IF NOT EXISTS idx_deployment_jobs_lease_until ON deployment_jobs(lease_until);

CREATE TABLE IF NOT EXISTS deployment_job_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL REFERENCES deployment_jobs(id) ON DELETE CASCADE,
    sequence integer NOT NULL,
    event_type varchar(64) NOT NULL,
    step varchar(64),
    status varchar(32),
    progress integer NOT NULL DEFAULT 0,
    public_message text,
    internal_message text,
    detail_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(job_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_deployment_job_events_job_id_id ON deployment_job_events(job_id, id);

CREATE TABLE IF NOT EXISTS audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    action varchar(128) NOT NULL,
    target_type varchar(64),
    target_id varchar(128),
    ip inet,
    user_agent text,
    detail_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_admin_user_id ON audit_logs(admin_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

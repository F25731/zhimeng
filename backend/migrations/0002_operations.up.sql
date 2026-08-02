CREATE TABLE IF NOT EXISTS site_heartbeats (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    version varchar(64), app_status varchar(32), worker_status varchar(32), database_status varchar(32),
    running_tasks integer NOT NULL DEFAULT 0, reported_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_site_heartbeats_site_received ON site_heartbeats(site_id, received_at DESC);

CREATE TABLE IF NOT EXISTS site_metric_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    users_total bigint NOT NULL DEFAULT 0, users_active bigint NOT NULL DEFAULT 0, calls_today bigint NOT NULL DEFAULT 0,
    calls_7d bigint NOT NULL DEFAULT 0, calls_lifetime bigint NOT NULL DEFAULT 0, success_7d bigint NOT NULL DEFAULT 0,
    failed_7d bigint NOT NULL DEFAULT 0, active_users_7d bigint NOT NULL DEFAULT 0, success_rate numeric NOT NULL DEFAULT 0,
    model_distribution_json jsonb NOT NULL DEFAULT '{}'::jsonb, reported_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_site_metric_snapshots_site_received ON site_metric_snapshots(site_id, received_at DESC);

CREATE TABLE IF NOT EXISTS site_channel_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    config_revision varchar(64), channels_json jsonb NOT NULL DEFAULT '[]'::jsonb, healthy_channels integer NOT NULL DEFAULT 0,
    total_channels integer NOT NULL DEFAULT 0, reported_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_site_channel_snapshots_site_received ON site_channel_snapshots(site_id, received_at DESC);

CREATE TABLE IF NOT EXISTS site_config_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    event_type varchar(64) NOT NULL, config_revision varchar(64), summary_json jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS release_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), version varchar(64) NOT NULL UNIQUE, image text NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'draft', channel varchar(32) NOT NULL DEFAULT 'stable', migration_version varchar(64),
    min_upgrade_version varchar(64), release_notes text, force_upgrade boolean NOT NULL DEFAULT false,
    published_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_release_versions_status ON release_versions(status);

CREATE TABLE IF NOT EXISTS site_upgrade_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    from_version varchar(64), to_version varchar(64) NOT NULL, job_id uuid REFERENCES deployment_jobs(id), status varchar(32) NOT NULL,
    backup_path text, started_at timestamptz, finished_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_site_upgrade_records_site ON site_upgrade_records(site_id, created_at DESC);

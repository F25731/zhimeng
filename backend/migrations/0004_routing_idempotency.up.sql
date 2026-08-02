ALTER TABLE domain_reservations
    ADD COLUMN IF NOT EXISTS consumed_at timestamptz;

ALTER TABLE provision_sessions
    ADD COLUMN IF NOT EXISTS reservation_id uuid REFERENCES domain_reservations(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS bound_site_id uuid REFERENCES sites(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS bound_job_id uuid REFERENCES deployment_jobs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_provision_sessions_bound_job
    ON provision_sessions(bound_job_id);

ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS route_status varchar(32) NOT NULL DEFAULT 'disabled',
    ADD COLUMN IF NOT EXISTS route_error text,
    ADD COLUMN IF NOT EXISTS route_verified_at timestamptz;

UPDATE sites
SET route_status='unverified'
WHERE status IN ('active','warning','offline','upgrading')
  AND route_status='disabled';

ALTER TABLE deployment_jobs
    ADD COLUMN IF NOT EXISTS lease_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS internal_error_message text;

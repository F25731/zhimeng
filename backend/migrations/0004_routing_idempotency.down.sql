ALTER TABLE deployment_jobs
    DROP COLUMN IF EXISTS internal_error_message,
    DROP COLUMN IF EXISTS lease_version;

ALTER TABLE sites
    DROP COLUMN IF EXISTS route_verified_at,
    DROP COLUMN IF EXISTS route_error,
    DROP COLUMN IF EXISTS route_status;

DROP INDEX IF EXISTS idx_provision_sessions_bound_job;

ALTER TABLE provision_sessions
    DROP COLUMN IF EXISTS bound_job_id,
    DROP COLUMN IF EXISTS bound_site_id,
    DROP COLUMN IF EXISTS reservation_id;

ALTER TABLE domain_reservations
    DROP COLUMN IF EXISTS consumed_at;

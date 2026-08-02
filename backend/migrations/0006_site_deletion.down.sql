DROP INDEX IF EXISTS idx_active_site_operation;
CREATE UNIQUE INDEX idx_active_site_operation
    ON deployment_jobs(site_id)
    WHERE status IN ('pending', 'running')
      AND job_type IN ('start','stop','restart','freeze','unfreeze','backup','upgrade');

ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS bootstrap_profile jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE sites
    ADD CONSTRAINT sites_bootstrap_profile_object
    CHECK (jsonb_typeof(bootstrap_profile) = 'object');

ALTER TABLE deployments ADD COLUMN exit_code INTEGER;
ALTER TABLE deployments ADD COLUMN log_tail TEXT;
ALTER TABLE deployments ADD COLUMN baseline_restart_count INTEGER NOT NULL DEFAULT 0;

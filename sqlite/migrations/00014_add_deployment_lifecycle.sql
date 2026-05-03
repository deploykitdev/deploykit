ALTER TABLE deployments ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE deployments ADD COLUMN failure_reason TEXT;
ALTER TABLE deployments ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN started_at TEXT;
ALTER TABLE deployments ADD COLUMN healthy_at TEXT;

CREATE INDEX idx_deployments_status ON deployments(status);

UPDATE deployments SET status = 'healthy', healthy_at = created_at
  WHERE id IN (SELECT active_deployment_id FROM services WHERE active_deployment_id IS NOT NULL);
UPDATE deployments SET status = 'superseded' WHERE status = 'pending';

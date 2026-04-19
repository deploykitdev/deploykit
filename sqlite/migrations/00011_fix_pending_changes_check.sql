-- 00010 originally shipped with a CHECK constraint requiring target_id or
-- target_temp_id to be present. That rejected env_var.create entries staged
-- under a pending-added service, which use parent_temp_id alone. SQLite can't
-- alter a CHECK in place, so rebuild the table with the correct constraint
-- and copy the existing rows over.

CREATE TABLE pending_changes_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq             INTEGER NOT NULL,
    op              TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT,
    target_temp_id  TEXT,
    parent_temp_id  TEXT,
    payload         TEXT NOT NULL DEFAULT '{}',
    user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (target_id IS NOT NULL OR target_temp_id IS NOT NULL OR parent_temp_id IS NOT NULL),
    UNIQUE (project_id, seq)
);

INSERT INTO pending_changes_new
    (id, project_id, seq, op, target_type, target_id, target_temp_id, parent_temp_id, payload, user_id, created_at)
SELECT id, project_id, seq, op, target_type, target_id, target_temp_id, parent_temp_id, payload, user_id, created_at
FROM pending_changes;

DROP TABLE pending_changes;

ALTER TABLE pending_changes_new RENAME TO pending_changes;

CREATE INDEX idx_pending_changes_project_seq ON pending_changes(project_id, seq);

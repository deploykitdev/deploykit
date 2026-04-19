-- Pending changes: an append-only operation log scoped to a project.
-- Entries are written when users edit anything deploy-impacting (service
-- create/update/delete, env var changes, project rename). Nothing mutates
-- the real services/env_vars/projects tables until the user clicks Deploy,
-- at which point all entries for a project apply atomically in a single
-- transaction and are then cleared.
CREATE TABLE pending_changes (
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

CREATE INDEX idx_pending_changes_project_seq ON pending_changes(project_id, seq);

-- Adds 'group' as a third env var scope. Resolution chain becomes
-- project → group → service. Group-scoped env vars belong to a canvas group
-- node and are inherited by every service whose canvas node has parent_id
-- pointing at that group.
--
-- SQLite can't ALTER an existing CHECK constraint, so we rebuild the table
-- with the widened constraint and re-create the cascade triggers. A new
-- trigger cleans up group env vars when the canvas group node is deleted.

PRAGMA foreign_keys = OFF;

CREATE TABLE env_vars_new (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL CHECK (scope IN ('project', 'group', 'service')),
    scope_id   TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (scope, scope_id, key)
);

INSERT INTO env_vars_new (id, scope, scope_id, key, value, created_at, updated_at)
SELECT id, scope, scope_id, key, value, created_at, updated_at FROM env_vars;

DROP TRIGGER IF EXISTS env_vars_cascade_project_delete;
DROP TRIGGER IF EXISTS env_vars_cascade_service_delete;
DROP INDEX IF EXISTS idx_env_vars_scope;
DROP TABLE env_vars;

ALTER TABLE env_vars_new RENAME TO env_vars;

CREATE INDEX idx_env_vars_scope ON env_vars(scope, scope_id);

CREATE TRIGGER env_vars_cascade_project_delete
AFTER DELETE ON projects
BEGIN
    DELETE FROM env_vars WHERE scope = 'project' AND scope_id = OLD.id;
END;

CREATE TRIGGER env_vars_cascade_service_delete
AFTER DELETE ON services
BEGIN
    DELETE FROM env_vars WHERE scope = 'service' AND scope_id = OLD.id;
END;

CREATE TRIGGER env_vars_cascade_group_delete
AFTER DELETE ON canvas_nodes
WHEN OLD.type = 'group'
BEGIN
    DELETE FROM env_vars WHERE scope = 'group' AND scope_id = OLD.id;
END;

PRAGMA foreign_keys = ON;

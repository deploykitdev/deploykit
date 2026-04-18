-- Environment variables for projects and services.
-- A single discriminated table so project- and service-scoped vars share one
-- code path. FK enforcement isn't possible across a discriminator, so cascade
-- deletes are handled via triggers on the owning tables.
CREATE TABLE env_vars (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL CHECK (scope IN ('project', 'service')),
    scope_id   TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (scope, scope_id, key)
);

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

-- System-wide settings stored as a key/value table. Used today for the
-- auto-update toggle; intentionally generic so future singletons (e.g.
-- "telemetry_enabled") can land here without another migration.
CREATE TABLE system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

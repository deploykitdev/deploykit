CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_hash TEXT NOT NULL UNIQUE,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    refresh_expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    prefix TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT,
    last_used_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

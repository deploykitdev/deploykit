package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/heyjorgedev/deploykit"
)

// SystemSettingsStore is a tiny key/value store for instance-wide settings.
// It is consumed by sysinfo.Service rather than being a domain service in
// its own right; the public surface lives on SystemService.
type SystemSettingsStore struct {
	db *DB
}

// NewSystemSettingsStore constructs a store backed by the given DB.
func NewSystemSettingsStore(db *DB) *SystemSettingsStore {
	return &SystemSettingsStore{db: db}
}

const (
	settingAutoUpdate = "auto_update"
)

// Get returns the current settings, applying defaults for keys that have
// never been written.
func (s *SystemSettingsStore) Get(ctx context.Context) (*deploykit.SystemSettings, error) {
	out := &deploykit.SystemSettings{
		AutoUpdate: false,
	}
	rows, err := s.db.db.QueryContext(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, fmt.Errorf("loading system settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning system setting: %w", err)
		}
		switch k {
		case settingAutoUpdate:
			out.AutoUpdate = v == "true"
		}
	}
	return out, rows.Err()
}

// Update applies a partial update. Nil pointers are left unchanged.
func (s *SystemSettingsStore) Update(ctx context.Context, u deploykit.SystemSettingsUpdate) (*deploykit.SystemSettings, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	if u.AutoUpdate != nil {
		if err := setSetting(ctx, tx, settingAutoUpdate, boolStr(*u.AutoUpdate)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing system settings: %w", err)
	}
	return s.Get(ctx)
}

func setSetting(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO system_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')
	`, key, value)
	if err != nil {
		return fmt.Errorf("upserting system setting %s: %w", key, err)
	}
	return nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

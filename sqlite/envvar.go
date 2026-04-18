package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/heyjorgedev/deploykit"
)

// EnvVarService implements deploykit.EnvVarService using SQLite.
type EnvVarService struct {
	db *DB
}

// NewEnvVarService creates a new EnvVarService backed by the given DB.
func NewEnvVarService(db *DB) *EnvVarService {
	return &EnvVarService{db: db}
}

func (s *EnvVarService) CreateEnvVar(ctx context.Context, scope deploykit.EnvVarScope, scopeID string, create deploykit.EnvVarCreate) (*deploykit.EnvVar, error) {
	if !scope.Valid() {
		return nil, deploykit.Errorf(deploykit.EINVALID, "Invalid env var scope.")
	}
	if err := create.Validate(); err != nil {
		return nil, err
	}

	// Verify the owning resource exists so we return ENOTFOUND rather than
	// silently creating an orphan row.
	if err := s.verifyScopeExists(ctx, scope, scopeID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ev := &deploykit.EnvVar{
		ID:        uuid.New().String(),
		Scope:     scope,
		ScopeID:   scopeID,
		Key:       create.Key,
		Value:     create.Value,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := s.db.db.ExecContext(ctx,
		`INSERT INTO env_vars (id, scope, scope_id, key, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, string(ev.Scope), ev.ScopeID, ev.Key, ev.Value,
		ev.CreatedAt.Format(timeFormat),
		ev.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, deploykit.Errorf(deploykit.ECONFLICT, "Env var %q already exists.", create.Key)
		}
		return nil, fmt.Errorf("creating env var: %w", err)
	}

	return ev, nil
}

func (s *EnvVarService) GetEnvVar(ctx context.Context, id string) (*deploykit.EnvVar, error) {
	ev := &deploykit.EnvVar{}
	var scope, createdAt, updatedAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, scope, scope_id, key, value, created_at, updated_at FROM env_vars WHERE id = ?`, id,
	).Scan(&ev.ID, &scope, &ev.ScopeID, &ev.Key, &ev.Value, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting env var %s: %w", id, err)
	}

	ev.Scope = deploykit.EnvVarScope(scope)
	ev.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	ev.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return ev, nil
}

func (s *EnvVarService) ListEnvVars(ctx context.Context, scope deploykit.EnvVarScope, scopeID string) ([]*deploykit.EnvVar, error) {
	if !scope.Valid() {
		return nil, deploykit.Errorf(deploykit.EINVALID, "Invalid env var scope.")
	}

	rows, err := s.db.db.QueryContext(ctx,
		`SELECT id, scope, scope_id, key, value, created_at, updated_at
		 FROM env_vars WHERE scope = ? AND scope_id = ? ORDER BY key ASC`,
		string(scope), scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing env vars: %w", err)
	}
	defer rows.Close()

	envVars := make([]*deploykit.EnvVar, 0)
	for rows.Next() {
		ev := &deploykit.EnvVar{}
		var scopeRaw, createdAt, updatedAt string
		if err := rows.Scan(&ev.ID, &scopeRaw, &ev.ScopeID, &ev.Key, &ev.Value, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning env var row: %w", err)
		}
		ev.Scope = deploykit.EnvVarScope(scopeRaw)
		ev.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		ev.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		envVars = append(envVars, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating env var rows: %w", err)
	}

	return envVars, nil
}

func (s *EnvVarService) UpdateEnvVar(ctx context.Context, id string, update deploykit.EnvVarUpdate) (*deploykit.EnvVar, error) {
	if err := update.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	ev := &deploykit.EnvVar{}
	var scope, createdAt, updatedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT id, scope, scope_id, key, value, created_at, updated_at FROM env_vars WHERE id = ?`, id,
	).Scan(&ev.ID, &scope, &ev.ScopeID, &ev.Key, &ev.Value, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting env var for update %s: %w", id, err)
	}
	ev.Scope = deploykit.EnvVarScope(scope)
	ev.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	if update.Value != nil {
		ev.Value = *update.Value
	}
	ev.UpdatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`UPDATE env_vars SET value = ?, updated_at = ? WHERE id = ?`,
		ev.Value, ev.UpdatedAt.Format(timeFormat), ev.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating env var %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing env var update: %w", err)
	}

	return ev, nil
}

func (s *EnvVarService) DeleteEnvVar(ctx context.Context, id string) error {
	result, err := s.db.db.ExecContext(ctx, `DELETE FROM env_vars WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting env var %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found.")
	}

	return nil
}

func (s *EnvVarService) ResolveForService(ctx context.Context, serviceID string) (map[string]string, error) {
	// Fetch the service's project ID.
	var projectID string
	err := s.db.db.QueryRowContext(ctx,
		`SELECT project_id FROM services WHERE id = ?`, serviceID,
	).Scan(&projectID)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting service %s: %w", serviceID, err)
	}

	// Fetch both scopes in one query. 'project' sorts before 'service'
	// alphabetically, so ORDER BY scope ASC returns project rows first and
	// service rows last — letting service values overwrite during the merge.
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT scope, key, value FROM env_vars
		 WHERE (scope = 'project' AND scope_id = ?) OR (scope = 'service' AND scope_id = ?)
		 ORDER BY scope ASC`,
		projectID, serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolving env vars for service %s: %w", serviceID, err)
	}
	defer rows.Close()

	merged := make(map[string]string)
	for rows.Next() {
		var scope, key, value string
		if err := rows.Scan(&scope, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning env var row: %w", err)
		}
		merged[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating env var rows: %w", err)
	}

	return merged, nil
}

// verifyScopeExists returns ENOTFOUND if the target project or service does
// not exist. Called before inserting a new env var.
func (s *EnvVarService) verifyScopeExists(ctx context.Context, scope deploykit.EnvVarScope, scopeID string) error {
	var query, notFoundMsg string
	switch scope {
	case deploykit.EnvVarScopeProject:
		query = `SELECT 1 FROM projects WHERE id = ?`
		notFoundMsg = "Project not found."
	case deploykit.EnvVarScopeService:
		query = `SELECT 1 FROM services WHERE id = ?`
		notFoundMsg = "Service not found."
	default:
		return deploykit.Errorf(deploykit.EINVALID, "Invalid env var scope.")
	}

	var exists bool
	err := s.db.db.QueryRowContext(ctx, query, scopeID).Scan(&exists)
	if err == sql.ErrNoRows {
		return deploykit.Errorf(deploykit.ENOTFOUND, "%s", notFoundMsg)
	} else if err != nil {
		return fmt.Errorf("checking scope %s %s: %w", scope, scopeID, err)
	}
	return nil
}

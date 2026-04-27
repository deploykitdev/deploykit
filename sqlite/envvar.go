package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/deploykitdev/deploykit"
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
	env, _, err := s.ResolveForServiceWithRefs(ctx, serviceID)
	return env, err
}

func (s *EnvVarService) ResolveForServiceWithRefs(ctx context.Context, serviceID string) (map[string]string, map[string][]string, error) {
	// Fetch the service's project ID + slug. Slug is needed to assemble
	// hostnames for `${{name.HOST}}` placeholder substitution.
	var projectID, projectSlug string
	err := s.db.db.QueryRowContext(ctx,
		`SELECT p.id, p.slug FROM services s JOIN projects p ON p.id = s.project_id WHERE s.id = ?`,
		serviceID,
	).Scan(&projectID, &projectSlug)
	if err == sql.ErrNoRows {
		return nil, nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, nil, fmt.Errorf("getting service %s: %w", serviceID, err)
	}

	// Build a name -> hostname lookup of every service in this project so we
	// can resolve placeholder references. We capture all services regardless
	// of deployment state — references to undeployed services still produce
	// the correct hostname; the consumer just won't be able to reach them
	// until the target is up.
	siblings, err := loadProjectServiceNames(ctx, s.db.db, projectID)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, fmt.Errorf("resolving env vars for service %s: %w", serviceID, err)
	}
	defer rows.Close()

	merged := make(map[string]string)
	for rows.Next() {
		var scope, key, value string
		if err := rows.Scan(&scope, &key, &value); err != nil {
			return nil, nil, fmt.Errorf("scanning env var row: %w", err)
		}
		merged[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating env var rows: %w", err)
	}

	resolved, refs := resolveServiceRefsInMap(merged, projectSlug, siblings)
	return resolved, refs, nil
}

// loadProjectServiceNames returns a set of all service names in a project.
// Used to validate `${{name.HOST}}` lookups during placeholder resolution.
func loadProjectServiceNames(ctx context.Context, db dbExecutor, projectID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM services WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing project services: %w", err)
	}
	defer rows.Close()
	names := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning service name: %w", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating service names: %w", err)
	}
	return names, nil
}

// resolveServiceRefsInMap walks every value in env, replacing `${{name.HOST}}`
// references with `dk-{projectSlug}-{name}-0` when the name is in `siblings`.
// Returns the resolved map and a per-key list of referenced service names.
func resolveServiceRefsInMap(env map[string]string, projectSlug string, siblings map[string]bool) (map[string]string, map[string][]string) {
	resolved := make(map[string]string, len(env))
	var refs map[string][]string
	for k, v := range env {
		newVal, names := deploykit.ResolveServiceRefs(v, func(name string) (string, bool) {
			if !siblings[name] {
				return "", false
			}
			return fmt.Sprintf("dk-%s-%s-0", projectSlug, name), true
		})
		resolved[k] = newVal
		if len(names) > 0 {
			if refs == nil {
				refs = make(map[string][]string)
			}
			refs[k] = names
		}
	}
	return resolved, refs
}

// dbExecutor abstracts over *sql.DB and *sql.Tx so helpers can be reused both
// outside and inside transactions.
type dbExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

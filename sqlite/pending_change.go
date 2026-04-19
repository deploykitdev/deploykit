package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/heyjorgedev/deploykit"
)

// PendingChangeService implements deploykit.PendingChangeService using SQLite.
type PendingChangeService struct {
	db *DB
}

// NewPendingChangeService creates a new PendingChangeService backed by the given DB.
func NewPendingChangeService(db *DB) *PendingChangeService {
	return &PendingChangeService{db: db}
}

func (s *PendingChangeService) List(ctx context.Context, projectID string) ([]*deploykit.PendingChange, error) {
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT id, project_id, seq, op, target_type, target_id, target_temp_id, parent_temp_id, payload, user_id, created_at
		 FROM pending_changes WHERE project_id = ? ORDER BY seq ASC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing pending changes: %w", err)
	}
	defer rows.Close()

	changes := make([]*deploykit.PendingChange, 0)
	for rows.Next() {
		pc, err := scanPendingChange(rows)
		if err != nil {
			return nil, err
		}
		changes = append(changes, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending change rows: %w", err)
	}

	return changes, nil
}

func (s *PendingChangeService) Append(ctx context.Context, projectID string, input deploykit.PendingChangeInput) (*deploykit.PendingChange, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify the project exists so we return ENOTFOUND rather than an orphan row.
	var ok bool
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE id = ?`, projectID).Scan(&ok)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	} else if err != nil {
		return nil, fmt.Errorf("checking project %s: %w", projectID, err)
	}

	var nextSeq int64
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM pending_changes WHERE project_id = ?`, projectID,
	).Scan(&nextSeq)
	if err != nil {
		return nil, fmt.Errorf("computing next seq: %w", err)
	}

	payload := input.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	pc := &deploykit.PendingChange{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		Seq:          nextSeq,
		Op:           input.Op,
		TargetType:   input.TargetType,
		TargetID:     input.TargetID,
		TargetTempID: input.TargetTempID,
		ParentTempID: input.ParentTempID,
		Payload:      payload,
		UserID:       input.UserID,
		CreatedAt:    time.Now().UTC(),
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO pending_changes (id, project_id, seq, op, target_type, target_id, target_temp_id, parent_temp_id, payload, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pc.ID, pc.ProjectID, pc.Seq, string(pc.Op), string(pc.TargetType),
		nullableString(pc.TargetID), nullableString(pc.TargetTempID), nullableString(pc.ParentTempID),
		string(pc.Payload), nullableString(pc.UserID),
		pc.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting pending change: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing pending change: %w", err)
	}

	return pc, nil
}

func (s *PendingChangeService) DiscardAll(ctx context.Context, projectID string) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning discard tx: %w", err)
	}
	defer tx.Rollback()

	// Pending-created services place a canvas node up-front (with
	// service_id = NULL) so the new node is visible to all clients before
	// deploy. Discarding must remove those nodes too, else they linger as
	// orphans pointing at a service that was never going to exist.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM canvas_nodes
		 WHERE project_id = ?
		   AND service_id IS NULL
		   AND id IN (
		     SELECT target_temp_id FROM pending_changes
		     WHERE project_id = ? AND op = ? AND target_temp_id IS NOT NULL
		   )`,
		projectID, projectID, string(deploykit.PendingOpServiceCreate),
	); err != nil {
		return fmt.Errorf("cleaning up pending-created canvas nodes: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pending_changes WHERE project_id = ?`, projectID,
	); err != nil {
		return fmt.Errorf("discarding pending changes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing discard: %w", err)
	}
	return nil
}

func (s *PendingChangeService) RemoveByTempID(ctx context.Context, projectID string, tempID string) ([]string, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM pending_changes
		 WHERE project_id = ? AND (target_temp_id = ? OR parent_temp_id = ?)`,
		projectID, tempID, tempID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing pending changes for temp id %s: %w", tempID, err)
	}
	var removed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning id: %w", err)
		}
		removed = append(removed, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ids: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pending_changes WHERE project_id = ? AND (target_temp_id = ? OR parent_temp_id = ?)`,
		projectID, tempID, tempID,
	); err != nil {
		return nil, fmt.Errorf("removing pending changes for temp id %s: %w", tempID, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing removal: %w", err)
	}

	return removed, nil
}

func (s *PendingChangeService) RemoveByID(ctx context.Context, projectID string, changeID string) error {
	result, err := s.db.db.ExecContext(ctx,
		`DELETE FROM pending_changes WHERE id = ? AND project_id = ?`,
		changeID, projectID,
	)
	if err != nil {
		return fmt.Errorf("removing pending change %s: %w", changeID, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Pending change not found.")
	}
	return nil
}

func (s *PendingChangeService) Apply(ctx context.Context, projectID string) (*deploykit.ApplyResult, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning apply transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify the project exists up front so we don't silently no-op on a bad ID.
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE id = ?`, projectID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	} else if err != nil {
		return nil, fmt.Errorf("checking project %s: %w", projectID, err)
	}

	// Load the full log in order.
	rows, err := tx.QueryContext(ctx,
		`SELECT id, project_id, seq, op, target_type, target_id, target_temp_id, parent_temp_id, payload, user_id, created_at
		 FROM pending_changes WHERE project_id = ? ORDER BY seq ASC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading pending changes: %w", err)
	}
	var entries []*deploykit.PendingChange
	for rows.Next() {
		pc, err := scanPendingChange(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, pc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending change rows: %w", err)
	}

	result := &deploykit.ApplyResult{
		TempIDToServiceID:    make(map[string]string),
		RedeployedServiceIDs: []string{},
		CreatedDeployments:   []*deploykit.Deployment{},
	}

	// Track services whose env var set changed so we can refresh their
	// deployment snapshot after applying all entries. Services that were
	// created in this same apply already got their initial deployment, so
	// they're excluded from refresh.
	affectedForRedeploy := make(map[string]bool)
	createdInApply := make(map[string]bool)

	for _, e := range entries {
		if err := s.applyEntry(ctx, tx, projectID, e, result, affectedForRedeploy, createdInApply); err != nil {
			return nil, fmt.Errorf("applying entry seq=%d op=%s: %w", e.Seq, e.Op, err)
		}
	}

	// Refresh deployment snapshots for affected (but not newly created) services.
	for svcID := range affectedForRedeploy {
		if createdInApply[svcID] {
			continue
		}
		dep, err := s.refreshDeploymentInTx(ctx, tx, svcID)
		if err != nil {
			return nil, fmt.Errorf("refreshing deployment for service %s: %w", svcID, err)
		}
		if dep != nil {
			result.RedeployedServiceIDs = append(result.RedeployedServiceIDs, svcID)
			result.CreatedDeployments = append(result.CreatedDeployments, dep)
		}
	}

	result.AppliedCount = len(entries)

	if _, err := tx.ExecContext(ctx, `DELETE FROM pending_changes WHERE project_id = ?`, projectID); err != nil {
		return nil, fmt.Errorf("clearing pending changes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing apply: %w", err)
	}

	return result, nil
}

// applyEntry mutates state for a single log entry inside the open tx.
func (s *PendingChangeService) applyEntry(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
	e *deploykit.PendingChange,
	result *deploykit.ApplyResult,
	affected map[string]bool,
	created map[string]bool,
) error {
	// Resolve the target ID: prefer the real ID; fall back to temp map.
	resolvedTarget := ""
	if e.TargetID != nil {
		resolvedTarget = *e.TargetID
	} else if e.TargetTempID != nil {
		if real, ok := result.TempIDToServiceID[*e.TargetTempID]; ok {
			resolvedTarget = real
		}
	}

	switch e.Op {
	case deploykit.PendingOpProjectUpdate:
		var p deploykit.ProjectUpdatePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("decoding project.update payload: %w", err)
		}
		if p.Name != nil {
			if *p.Name == "" {
				return deploykit.Errorf(deploykit.EINVALID, "Project name cannot be empty.")
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE projects SET name = ?, updated_at = ? WHERE id = ?`,
				*p.Name, time.Now().UTC().Format(timeFormat), projectID,
			); err != nil {
				return fmt.Errorf("updating project: %w", err)
			}
		}
		return nil

	case deploykit.PendingOpServiceCreate:
		if e.TargetTempID == nil {
			return fmt.Errorf("service.create missing target_temp_id")
		}
		var p deploykit.ServiceCreatePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("decoding service.create payload: %w", err)
		}
		if p.Name == "" {
			return deploykit.Errorf(deploykit.EINVALID, "Service name is required.")
		}
		if p.Image == "" {
			return deploykit.Errorf(deploykit.EINVALID, "Service image is required.")
		}

		svcID := uuid.New().String()
		now := time.Now().UTC()
		var iconURLArg any
		if p.IconURL != nil && *p.IconURL != "" {
			iconURLArg = *p.IconURL
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO services (id, project_id, name, status, icon_url, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			svcID, projectID, p.Name, deploykit.ServiceStatusCreated, iconURLArg,
			now.Format(timeFormat), now.Format(timeFormat),
		); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return deploykit.Errorf(deploykit.ECONFLICT, "A service named %q already exists in the project.", p.Name)
			}
			return fmt.Errorf("inserting service: %w", err)
		}

		// Insert baked-in env vars for this service.
		for _, ev := range p.EnvVars {
			if ev.Key == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO env_vars (id, scope, scope_id, key, value, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				uuid.New().String(), string(deploykit.EnvVarScopeService), svcID, ev.Key, ev.Value,
				now.Format(timeFormat), now.Format(timeFormat),
			); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return deploykit.Errorf(deploykit.ECONFLICT, "Env var %q is duplicated on service %q.", ev.Key, p.Name)
				}
				return fmt.Errorf("inserting service env var: %w", err)
			}
		}

		// Build the initial deployment with project+service env vars merged.
		resolved, err := resolveEnvVarsInTx(ctx, tx, projectID, svcID)
		if err != nil {
			return fmt.Errorf("resolving env vars: %w", err)
		}
		ports := p.Ports
		if ports == nil {
			ports = []deploykit.PortMapping{}
		}
		dep, err := createDeploymentInTx(ctx, tx, svcID, deploykit.DeploymentCreate{
			Image:    p.Image,
			EnvVars:  resolved,
			Ports:    ports,
			Replicas: 1,
		})
		if err != nil {
			return fmt.Errorf("creating initial deployment: %w", err)
		}

		// Link the pre-existing canvas node to the new service.
		data, _ := json.Marshal(map[string]any{"image": dep.Image})
		if _, err := tx.ExecContext(ctx,
			`UPDATE canvas_nodes
			 SET service_id = ?, label = ?, data = ?, updated_at = ?
			 WHERE id = ? AND project_id = ?`,
			svcID, p.Name, string(data), now.Format(timeFormat),
			*e.TargetTempID, projectID,
		); err != nil {
			return fmt.Errorf("linking canvas node to service: %w", err)
		}

		result.TempIDToServiceID[*e.TargetTempID] = svcID
		result.CreatedDeployments = append(result.CreatedDeployments, dep)
		created[svcID] = true
		return nil

	case deploykit.PendingOpServiceUpdate:
		if resolvedTarget == "" {
			return fmt.Errorf("service.update missing target")
		}
		var p deploykit.ServiceUpdatePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("decoding service.update payload: %w", err)
		}
		sets := []string{"updated_at = ?"}
		args := []any{time.Now().UTC().Format(timeFormat)}
		if p.Name != nil {
			if *p.Name == "" {
				return deploykit.Errorf(deploykit.EINVALID, "Service name cannot be empty.")
			}
			sets = append(sets, "name = ?")
			args = append(args, *p.Name)
		}
		if p.IconURL != nil {
			if *p.IconURL == "" {
				sets = append(sets, "icon_url = NULL")
			} else {
				sets = append(sets, "icon_url = ?")
				args = append(args, *p.IconURL)
			}
		}
		if len(sets) == 1 {
			return nil // nothing to update
		}
		args = append(args, resolvedTarget)
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE services SET %s WHERE id = ?`, strings.Join(sets, ", ")),
			args...,
		); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return deploykit.Errorf(deploykit.ECONFLICT, "A service with this name already exists in the project.")
			}
			return fmt.Errorf("updating service: %w", err)
		}
		return nil

	case deploykit.PendingOpServiceDelete:
		if resolvedTarget == "" {
			return fmt.Errorf("service.delete missing target")
		}
		// Remove the backing canvas node first (its service_id FK would null
		// out via ON DELETE SET NULL, but we also want the node gone).
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM canvas_nodes WHERE project_id = ? AND service_id = ?`,
			projectID, resolvedTarget,
		); err != nil {
			return fmt.Errorf("deleting canvas node for service: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM services WHERE id = ? AND project_id = ?`,
			resolvedTarget, projectID,
		); err != nil {
			return fmt.Errorf("deleting service: %w", err)
		}
		// Clear any redeploy mark — the service is gone.
		delete(affected, resolvedTarget)
		return nil

	case deploykit.PendingOpEnvVarCreate:
		var p deploykit.EnvVarCreatePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("decoding env_var.create payload: %w", err)
		}
		if !p.Scope.Valid() {
			return deploykit.Errorf(deploykit.EINVALID, "Invalid env var scope.")
		}
		if p.Key == "" {
			return deploykit.Errorf(deploykit.EINVALID, "Env var key is required.")
		}
		scopeID := ""
		if e.ParentTempID != nil {
			real, ok := result.TempIDToServiceID[*e.ParentTempID]
			if !ok {
				return fmt.Errorf("env_var.create references unknown parent temp id %s", *e.ParentTempID)
			}
			scopeID = real
		} else if e.TargetID != nil {
			scopeID = *e.TargetID
		} else {
			return fmt.Errorf("env_var.create missing target or parent")
		}
		now := time.Now().UTC().Format(timeFormat)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO env_vars (id, scope, scope_id, key, value, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), string(p.Scope), scopeID, p.Key, p.Value, now, now,
		); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return deploykit.Errorf(deploykit.ECONFLICT, "Env var %q already exists.", p.Key)
			}
			return fmt.Errorf("creating env var: %w", err)
		}
		if err := markEnvVarAffected(ctx, tx, projectID, p.Scope, scopeID, affected); err != nil {
			return err
		}
		return nil

	case deploykit.PendingOpEnvVarUpdate:
		if resolvedTarget == "" {
			return fmt.Errorf("env_var.update missing target")
		}
		var p deploykit.EnvVarUpdatePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("decoding env_var.update payload: %w", err)
		}
		var scope, scopeID string
		err := tx.QueryRowContext(ctx,
			`SELECT scope, scope_id FROM env_vars WHERE id = ?`, resolvedTarget,
		).Scan(&scope, &scopeID)
		if err == sql.ErrNoRows {
			return deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found.")
		} else if err != nil {
			return fmt.Errorf("loading env var for update: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE env_vars SET value = ?, updated_at = ? WHERE id = ?`,
			p.Value, time.Now().UTC().Format(timeFormat), resolvedTarget,
		); err != nil {
			return fmt.Errorf("updating env var: %w", err)
		}
		if err := markEnvVarAffected(ctx, tx, projectID, deploykit.EnvVarScope(scope), scopeID, affected); err != nil {
			return err
		}
		return nil

	case deploykit.PendingOpEnvVarDelete:
		if resolvedTarget == "" {
			return fmt.Errorf("env_var.delete missing target")
		}
		var scope, scopeID string
		err := tx.QueryRowContext(ctx,
			`SELECT scope, scope_id FROM env_vars WHERE id = ?`, resolvedTarget,
		).Scan(&scope, &scopeID)
		if err == sql.ErrNoRows {
			return deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found.")
		} else if err != nil {
			return fmt.Errorf("loading env var for delete: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM env_vars WHERE id = ?`, resolvedTarget,
		); err != nil {
			return fmt.Errorf("deleting env var: %w", err)
		}
		if err := markEnvVarAffected(ctx, tx, projectID, deploykit.EnvVarScope(scope), scopeID, affected); err != nil {
			return err
		}
		return nil

	default:
		return fmt.Errorf("unknown op %q", e.Op)
	}
}

// refreshDeploymentInTx reproduces redeployServiceNoTrigger's behaviour inside
// an open transaction. If the service has no active deployment, returns
// (nil, nil) — first-time deploys happen through service.create, not here.
func (s *PendingChangeService) refreshDeploymentInTx(ctx context.Context, tx *sql.Tx, serviceID string) (*deploykit.Deployment, error) {
	// Load active deployment for this service.
	var activeDepID sql.NullString
	var projectID string
	if err := tx.QueryRowContext(ctx,
		`SELECT project_id, active_deployment_id FROM services WHERE id = ?`, serviceID,
	).Scan(&projectID, &activeDepID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("loading service %s: %w", serviceID, err)
	}
	if !activeDepID.Valid {
		return nil, nil
	}

	// Pull the existing deployment snapshot fields.
	var image string
	var envRaw, portsRaw string
	var resourcesRaw sql.NullString
	var replicas int
	if err := tx.QueryRowContext(ctx,
		`SELECT image, env_vars, ports, resources, replicas FROM deployments WHERE id = ?`,
		activeDepID.String,
	).Scan(&image, &envRaw, &portsRaw, &resourcesRaw, &replicas); err != nil {
		return nil, fmt.Errorf("loading deployment %s: %w", activeDepID.String, err)
	}

	var ports []deploykit.PortMapping
	if err := json.Unmarshal([]byte(portsRaw), &ports); err != nil {
		return nil, fmt.Errorf("unmarshaling ports: %w", err)
	}
	var resources *deploykit.ResourceLimits
	if resourcesRaw.Valid {
		resources = &deploykit.ResourceLimits{}
		if err := json.Unmarshal([]byte(resourcesRaw.String), resources); err != nil {
			return nil, fmt.Errorf("unmarshaling resources: %w", err)
		}
	}

	resolved, err := resolveEnvVarsInTx(ctx, tx, projectID, serviceID)
	if err != nil {
		return nil, fmt.Errorf("resolving env vars: %w", err)
	}

	return createDeploymentInTx(ctx, tx, serviceID, deploykit.DeploymentCreate{
		Image:     image,
		EnvVars:   resolved,
		Ports:     ports,
		Resources: resources,
		Replicas:  replicas,
	})
}

// markEnvVarAffected records which services need their deployment refreshed.
// Project-scoped env var changes fan out to every service in the project.
func markEnvVarAffected(ctx context.Context, tx *sql.Tx, projectID string, scope deploykit.EnvVarScope, scopeID string, affected map[string]bool) error {
	switch scope {
	case deploykit.EnvVarScopeService:
		affected[scopeID] = true
	case deploykit.EnvVarScopeProject:
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM services WHERE project_id = ? AND active_deployment_id IS NOT NULL`,
			projectID,
		)
		if err != nil {
			return fmt.Errorf("listing project services: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("scanning service id: %w", err)
			}
			affected[id] = true
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterating service rows: %w", err)
		}
	}
	return nil
}

// resolveEnvVarsInTx merges project and service env vars for the given service
// inside an open transaction. Service values override project values.
func resolveEnvVarsInTx(ctx context.Context, tx *sql.Tx, projectID, serviceID string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT scope, key, value FROM env_vars
		 WHERE (scope = 'project' AND scope_id = ?) OR (scope = 'service' AND scope_id = ?)
		 ORDER BY scope ASC`,
		projectID, serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolving env vars: %w", err)
	}
	defer rows.Close()

	merged := make(map[string]string)
	for rows.Next() {
		var scope, key, value string
		if err := rows.Scan(&scope, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning env var: %w", err)
		}
		merged[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating env vars: %w", err)
	}
	return merged, nil
}

// createDeploymentInTx mirrors DeploymentService.CreateDeployment but runs
// inside a caller-owned transaction. Returns the created deployment.
func createDeploymentInTx(ctx context.Context, tx *sql.Tx, serviceID string, create deploykit.DeploymentCreate) (*deploykit.Deployment, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	replicas := create.Replicas
	if replicas == 0 {
		replicas = 1
	}
	envVars := create.EnvVars
	if envVars == nil {
		envVars = map[string]string{}
	}
	ports := create.Ports
	if ports == nil {
		ports = []deploykit.PortMapping{}
	}

	envJSON, err := json.Marshal(envVars)
	if err != nil {
		return nil, fmt.Errorf("marshaling env vars: %w", err)
	}
	portsJSON, err := json.Marshal(ports)
	if err != nil {
		return nil, fmt.Errorf("marshaling ports: %w", err)
	}
	var resourcesArg any
	if create.Resources != nil {
		b, err := json.Marshal(create.Resources)
		if err != nil {
			return nil, fmt.Errorf("marshaling resources: %w", err)
		}
		resourcesArg = string(b)
	}

	dep := &deploykit.Deployment{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Image:     create.Image,
		EnvVars:   envVars,
		Ports:     ports,
		Resources: create.Resources,
		Replicas:  replicas,
		CreatedAt: time.Now().UTC(),
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO deployments (id, service_id, image, env_vars, ports, resources, replicas, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		dep.ID, dep.ServiceID, dep.Image,
		string(envJSON), string(portsJSON), resourcesArg,
		dep.Replicas, dep.CreatedAt.Format(timeFormat),
	); err != nil {
		return nil, fmt.Errorf("inserting deployment: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE services SET active_deployment_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		dep.ID, deploykit.ServiceStatusDeploying,
		time.Now().UTC().Format(timeFormat), serviceID,
	); err != nil {
		return nil, fmt.Errorf("activating deployment on service: %w", err)
	}

	return dep, nil
}

// scanPendingChange scans a row into a PendingChange.
func scanPendingChange(rows *sql.Rows) (*deploykit.PendingChange, error) {
	pc := &deploykit.PendingChange{}
	var opStr, targetTypeStr, payload, createdAt string
	var targetID, targetTempID, parentTempID, userID sql.NullString

	if err := rows.Scan(
		&pc.ID, &pc.ProjectID, &pc.Seq, &opStr, &targetTypeStr,
		&targetID, &targetTempID, &parentTempID,
		&payload, &userID, &createdAt,
	); err != nil {
		return nil, fmt.Errorf("scanning pending change: %w", err)
	}
	pc.Op = deploykit.PendingChangeOp(opStr)
	pc.TargetType = deploykit.PendingChangeTarget(targetTypeStr)
	if targetID.Valid {
		pc.TargetID = &targetID.String
	}
	if targetTempID.Valid {
		pc.TargetTempID = &targetTempID.String
	}
	if parentTempID.Valid {
		pc.ParentTempID = &parentTempID.String
	}
	if userID.Valid {
		pc.UserID = &userID.String
	}
	pc.Payload = json.RawMessage(payload)
	pc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return pc, nil
}

// nullableString wraps a *string for SQL insertion: nil -> NULL.
func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

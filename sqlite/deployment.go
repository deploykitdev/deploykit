package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/deploykitdev/deploykit"
)

// DeploymentService implements deploykit.DeploymentService using SQLite.
type DeploymentService struct {
	db *DB
}

// NewDeploymentService creates a new DeploymentService backed by the given DB.
func NewDeploymentService(db *DB) *DeploymentService {
	return &DeploymentService{db: db}
}

// deploymentColumns is the SELECT list used by every read; keeping it in one
// place ensures GetDeployment/ListDeployments/ListInFlightDeployments stay aligned.
const deploymentColumns = `id, service_id, image, env_vars, ports, resources, replicas, status, failure_reason, exit_code, log_tail, baseline_restart_count, attempt_count, started_at, healthy_at, created_at`

func (s *DeploymentService) CreateDeployment(ctx context.Context, serviceID string, create deploykit.DeploymentCreate) (*deploykit.Deployment, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify service exists and capture whether it has an active deployment;
	// we only flip service.status to "deploying" for first deploys.
	var activeDepID sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT active_deployment_id FROM services WHERE id = ?`, serviceID,
	).Scan(&activeDepID)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("checking service %s: %w", serviceID, err)
	}

	// Default replicas to 1.
	replicas := create.Replicas
	if replicas == 0 {
		replicas = 1
	}

	// Default env vars to empty map.
	envVars := create.EnvVars
	if envVars == nil {
		envVars = map[string]string{}
	}

	// Default ports to empty slice.
	ports := create.Ports
	if ports == nil {
		ports = []deploykit.PortMapping{}
	}

	envVarsJSON, err := json.Marshal(envVars)
	if err != nil {
		return nil, fmt.Errorf("marshaling env vars: %w", err)
	}

	portsJSON, err := json.Marshal(ports)
	if err != nil {
		return nil, fmt.Errorf("marshaling ports: %w", err)
	}

	var resourcesJSON []byte
	if create.Resources != nil {
		resourcesJSON, err = json.Marshal(create.Resources)
		if err != nil {
			return nil, fmt.Errorf("marshaling resources: %w", err)
		}
	}

	deployment := &deploykit.Deployment{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Image:     create.Image,
		EnvVars:   envVars,
		Ports:     ports,
		Resources: create.Resources,
		Replicas:  replicas,
		Status:    deploykit.DeploymentStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	var resourcesArg any
	if resourcesJSON != nil {
		resourcesArg = string(resourcesJSON)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO deployments (id, service_id, image, env_vars, ports, resources, replicas, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deployment.ID, deployment.ServiceID, deployment.Image,
		string(envVarsJSON), string(portsJSON), resourcesArg,
		deployment.Replicas, deployment.Status,
		deployment.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("creating deployment: %w", err)
	}

	// Cancel any previously-pending or starting deployment for this service —
	// the reconciler should focus on the latest attempt.
	_, err = tx.ExecContext(ctx,
		`UPDATE deployments SET status = ?
		 WHERE service_id = ? AND id != ? AND status IN (?, ?)`,
		deploykit.DeploymentStatusCancelled, serviceID, deployment.ID,
		deploykit.DeploymentStatusPending, deploykit.DeploymentStatusStarting,
	)
	if err != nil {
		return nil, fmt.Errorf("cancelling prior in-flight deployments: %w", err)
	}

	// First-deploy UX: when the service has no active deployment, surface
	// "deploying" on the service row. For subsequent redeploys, leave service
	// status alone — the prior healthy deployment is still serving.
	if !activeDepID.Valid {
		_, err = tx.ExecContext(ctx,
			`UPDATE services SET status = ?, updated_at = ? WHERE id = ?`,
			deploykit.ServiceStatusDeploying,
			time.Now().UTC().Format(timeFormat), serviceID,
		)
		if err != nil {
			return nil, fmt.Errorf("setting service status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing deployment: %w", err)
	}

	return deployment, nil
}

// scanDeployment scans one row from a SELECT that uses deploymentColumns.
func scanDeployment(row interface {
	Scan(dest ...any) error
}, d *deploykit.Deployment) error {
	var createdAt string
	var envVarsRaw, portsRaw, status string
	var resourcesRaw, failureReason, logTail, startedAt, healthyAt sql.NullString
	var exitCode sql.NullInt64
	if err := row.Scan(
		&d.ID, &d.ServiceID, &d.Image,
		&envVarsRaw, &portsRaw, &resourcesRaw, &d.Replicas,
		&status, &failureReason, &exitCode, &logTail, &d.BaselineRestartCount,
		&d.AttemptCount, &startedAt, &healthyAt,
		&createdAt,
	); err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(envVarsRaw), &d.EnvVars); err != nil {
		return fmt.Errorf("unmarshaling env vars: %w", err)
	}
	if err := json.Unmarshal([]byte(portsRaw), &d.Ports); err != nil {
		return fmt.Errorf("unmarshaling ports: %w", err)
	}
	if resourcesRaw.Valid {
		d.Resources = &deploykit.ResourceLimits{}
		if err := json.Unmarshal([]byte(resourcesRaw.String), d.Resources); err != nil {
			return fmt.Errorf("unmarshaling resources: %w", err)
		}
	}
	d.Status = status
	if failureReason.Valid {
		s := failureReason.String
		d.FailureReason = &s
	}
	if exitCode.Valid {
		ec := int(exitCode.Int64)
		d.ExitCode = &ec
	}
	if logTail.Valid {
		s := logTail.String
		d.LogTail = &s
	}
	if startedAt.Valid {
		t, _ := time.Parse(timeFormat, startedAt.String)
		d.StartedAt = &t
	}
	if healthyAt.Valid {
		t, _ := time.Parse(timeFormat, healthyAt.String)
		d.HealthyAt = &t
	}
	d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return nil
}

func (s *DeploymentService) GetDeployment(ctx context.Context, id string) (*deploykit.Deployment, error) {
	d := &deploykit.Deployment{}
	row := s.db.db.QueryRowContext(ctx,
		`SELECT `+deploymentColumns+` FROM deployments WHERE id = ?`, id,
	)
	if err := scanDeployment(row, d); err != nil {
		if err == sql.ErrNoRows {
			return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
		}
		return nil, fmt.Errorf("getting deployment %s: %w", id, err)
	}
	return d, nil
}

func (s *DeploymentService) ListDeployments(ctx context.Context, filter deploykit.DeploymentFilter) ([]*deploykit.Deployment, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.ServiceID != nil {
		where = append(where, "service_id = ?")
		args = append(args, *filter.ServiceID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(
		`SELECT `+deploymentColumns+`, COUNT(*) OVER() AS total_count
		 FROM deployments WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)
	args = append(args, limit, offset)

	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*deploykit.Deployment
	var totalCount int

	for rows.Next() {
		d := &deploykit.Deployment{}
		// Wrap rows so we can append totalCount to the scan target list.
		var createdAt string
		var envVarsRaw, portsRaw, status string
		var resourcesRaw, failureReason, logTail, startedAt, healthyAt sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(
			&d.ID, &d.ServiceID, &d.Image,
			&envVarsRaw, &portsRaw, &resourcesRaw, &d.Replicas,
			&status, &failureReason, &exitCode, &logTail, &d.BaselineRestartCount,
			&d.AttemptCount, &startedAt, &healthyAt,
			&createdAt, &totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning deployment row: %w", err)
		}
		if err := json.Unmarshal([]byte(envVarsRaw), &d.EnvVars); err != nil {
			return nil, 0, fmt.Errorf("unmarshaling env vars: %w", err)
		}
		if err := json.Unmarshal([]byte(portsRaw), &d.Ports); err != nil {
			return nil, 0, fmt.Errorf("unmarshaling ports: %w", err)
		}
		if resourcesRaw.Valid {
			d.Resources = &deploykit.ResourceLimits{}
			if err := json.Unmarshal([]byte(resourcesRaw.String), d.Resources); err != nil {
				return nil, 0, fmt.Errorf("unmarshaling resources: %w", err)
			}
		}
		d.Status = status
		if failureReason.Valid {
			fr := failureReason.String
			d.FailureReason = &fr
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			d.ExitCode = &ec
		}
		if logTail.Valid {
			lt := logTail.String
			d.LogTail = &lt
		}
		if startedAt.Valid {
			t, _ := time.Parse(timeFormat, startedAt.String)
			d.StartedAt = &t
		}
		if healthyAt.Valid {
			t, _ := time.Parse(timeFormat, healthyAt.String)
			d.HealthyAt = &t
		}
		d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		deployments = append(deployments, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating deployment rows: %w", err)
	}

	return deployments, totalCount, nil
}

func (s *DeploymentService) ListInFlightDeployments(ctx context.Context) ([]*deploykit.Deployment, error) {
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT `+deploymentColumns+` FROM deployments
		 WHERE status IN (?, ?, ?)
		 ORDER BY created_at ASC`,
		deploykit.DeploymentStatusPending,
		deploykit.DeploymentStatusStarting,
		deploykit.DeploymentStatusHealthy,
	)
	if err != nil {
		return nil, fmt.Errorf("listing in-flight deployments: %w", err)
	}
	defer rows.Close()

	var out []*deploykit.Deployment
	for rows.Next() {
		d := &deploykit.Deployment{}
		if err := scanDeployment(rows, d); err != nil {
			return nil, fmt.Errorf("scanning in-flight deployment: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating in-flight deployment rows: %w", err)
	}
	return out, nil
}

func (s *DeploymentService) MarkDeploymentStarting(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.db.ExecContext(ctx,
		`UPDATE deployments SET status = ?, started_at = COALESCE(started_at, ?)
		 WHERE id = ? AND status = ?`,
		deploykit.DeploymentStatusStarting, now, id, deploykit.DeploymentStatusPending,
	)
	if err != nil {
		return fmt.Errorf("marking deployment %s starting: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Either already starting/healthy/failed/cancelled, or doesn't exist.
		// Verify existence — if missing, surface ENOTFOUND. Otherwise treat as no-op.
		var exists int
		if err := s.db.db.QueryRowContext(ctx, `SELECT 1 FROM deployments WHERE id = ?`, id).Scan(&exists); err == sql.ErrNoRows {
			return deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
		}
	}
	return nil
}

func (s *DeploymentService) MarkDeploymentHealthy(ctx context.Context, id string, baselineRestartCount int) (string, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var serviceID string
	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT service_id, status FROM deployments WHERE id = ?`, id,
	).Scan(&serviceID, &status)
	if err == sql.ErrNoRows {
		return "", deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
	} else if err != nil {
		return "", fmt.Errorf("looking up deployment %s: %w", id, err)
	}
	if status == deploykit.DeploymentStatusHealthy {
		// Already healthy — likely a re-entrant call. Nothing to do.
		return "", tx.Commit()
	}

	var priorActive sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT active_deployment_id FROM services WHERE id = ?`, serviceID,
	).Scan(&priorActive)
	if err != nil {
		return "", fmt.Errorf("looking up service %s: %w", serviceID, err)
	}

	now := time.Now().UTC().Format(timeFormat)

	_, err = tx.ExecContext(ctx,
		`UPDATE deployments SET status = ?, healthy_at = ?, baseline_restart_count = ?, failure_reason = NULL
		 WHERE id = ?`,
		deploykit.DeploymentStatusHealthy, now, baselineRestartCount, id,
	)
	if err != nil {
		return "", fmt.Errorf("marking deployment healthy: %w", err)
	}

	priorActiveID := ""
	if priorActive.Valid && priorActive.String != id {
		priorActiveID = priorActive.String
		_, err = tx.ExecContext(ctx,
			`UPDATE deployments SET status = ? WHERE id = ? AND status = ?`,
			deploykit.DeploymentStatusSuperseded, priorActiveID, deploykit.DeploymentStatusHealthy,
		)
		if err != nil {
			return "", fmt.Errorf("superseding prior deployment %s: %w", priorActiveID, err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE services SET active_deployment_id = ?, updated_at = ? WHERE id = ?`,
		id, now, serviceID,
	)
	if err != nil {
		return "", fmt.Errorf("flipping service active deployment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing healthy transition: %w", err)
	}

	return priorActiveID, nil
}

// logTailMaxBytes caps log_tail storage to keep deployment rows compact.
const logTailMaxBytes = 10 * 1024

func (s *DeploymentService) MarkDeploymentFailed(ctx context.Context, id string, reason string, exitCode *int, logTail string) error {
	// Truncate from the front so the panic / final lines (the useful part)
	// survive the cap.
	if len(logTail) > logTailMaxBytes {
		logTail = logTail[len(logTail)-logTailMaxBytes:]
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var serviceID string
	err = tx.QueryRowContext(ctx,
		`SELECT service_id FROM deployments WHERE id = ?`, id,
	).Scan(&serviceID)
	if err == sql.ErrNoRows {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
	} else if err != nil {
		return fmt.Errorf("looking up deployment %s: %w", id, err)
	}

	var exitCodeArg any
	if exitCode != nil {
		exitCodeArg = *exitCode
	}
	var logTailArg any
	if logTail != "" {
		logTailArg = logTail
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE deployments SET status = ?, failure_reason = ?, exit_code = ?, log_tail = ? WHERE id = ?`,
		deploykit.DeploymentStatusFailed, reason, exitCodeArg, logTailArg, id,
	)
	if err != nil {
		return fmt.Errorf("marking deployment failed: %w", err)
	}

	// If the service has no other healthy deployment to fall back on, flip
	// service status to "failed" too. Otherwise the prior healthy keeps
	// serving and we leave service.status alone.
	var hasHealthy bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM deployments WHERE service_id = ? AND status = ?)`,
		serviceID, deploykit.DeploymentStatusHealthy,
	).Scan(&hasHealthy)
	if err != nil {
		return fmt.Errorf("checking for healthy deployments: %w", err)
	}
	if !hasHealthy {
		_, err = tx.ExecContext(ctx,
			`UPDATE services SET status = ?, updated_at = ? WHERE id = ?`,
			deploykit.ServiceStatusFailed, time.Now().UTC().Format(timeFormat), serviceID,
		)
		if err != nil {
			return fmt.Errorf("flipping service status to failed: %w", err)
		}
	}

	return tx.Commit()
}

func (s *DeploymentService) IncrementDeploymentAttempt(ctx context.Context, id string) (int, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE deployments SET attempt_count = attempt_count + 1 WHERE id = ?`, id,
	)
	if err != nil {
		return 0, fmt.Errorf("incrementing attempt for %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
	}

	var newCount int
	if err := tx.QueryRowContext(ctx, `SELECT attempt_count FROM deployments WHERE id = ?`, id).Scan(&newCount); err != nil {
		return 0, fmt.Errorf("reading back attempt_count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing attempt increment: %w", err)
	}
	return newCount, nil
}

func (s *DeploymentService) RollbackService(ctx context.Context, serviceID string, deploymentID string) (*deploykit.Service, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify the deployment exists and belongs to the service.
	var depServiceID string
	err = tx.QueryRowContext(ctx,
		`SELECT service_id FROM deployments WHERE id = ?`, deploymentID,
	).Scan(&depServiceID)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
	} else if err != nil {
		return nil, fmt.Errorf("checking deployment %s: %w", deploymentID, err)
	}
	if depServiceID != serviceID {
		return nil, deploykit.Errorf(deploykit.EINVALID, "Deployment does not belong to this service.")
	}

	var priorActive sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT active_deployment_id FROM services WHERE id = ?`, serviceID,
	).Scan(&priorActive); err != nil {
		return nil, fmt.Errorf("looking up service: %w", err)
	}

	now := time.Now().UTC().Format(timeFormat)

	// The rolled-back-to deployment becomes healthy again.
	_, err = tx.ExecContext(ctx,
		`UPDATE deployments SET status = ?, healthy_at = COALESCE(healthy_at, ?) WHERE id = ?`,
		deploykit.DeploymentStatusHealthy, now, deploymentID,
	)
	if err != nil {
		return nil, fmt.Errorf("marking rollback target healthy: %w", err)
	}

	// The previously-active deployment is superseded (only if it was healthy).
	if priorActive.Valid && priorActive.String != deploymentID {
		_, err = tx.ExecContext(ctx,
			`UPDATE deployments SET status = ? WHERE id = ? AND status = ?`,
			deploykit.DeploymentStatusSuperseded, priorActive.String, deploykit.DeploymentStatusHealthy,
		)
		if err != nil {
			return nil, fmt.Errorf("superseding prior active deployment: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE services SET active_deployment_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		deploymentID, deploykit.ServiceStatusDeploying, now, serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating service for rollback: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing rollback: %w", err)
	}

	// Read back the updated service.
	svc := &deploykit.Service{}
	var createdAt, updatedAt string
	var activeDepID sql.NullString
	err = s.db.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, status, active_deployment_id, created_at, updated_at FROM services WHERE id = ?`, serviceID,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &activeDepID, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("reading back service after rollback: %w", err)
	}
	if activeDepID.Valid {
		svc.ActiveDeploymentID = &activeDepID.String
	}
	svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	svc.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return svc, nil
}

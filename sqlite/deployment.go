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

// DeploymentService implements deploykit.DeploymentService using SQLite.
type DeploymentService struct {
	db *DB
}

// NewDeploymentService creates a new DeploymentService backed by the given DB.
func NewDeploymentService(db *DB) *DeploymentService {
	return &DeploymentService{db: db}
}

func (s *DeploymentService) CreateDeployment(ctx context.Context, serviceID string, create deploykit.DeploymentCreate) (*deploykit.Deployment, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify service exists.
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM services WHERE id = ?`, serviceID).Scan(&exists)
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
		CreatedAt: time.Now().UTC(),
	}

	var resourcesArg any
	if resourcesJSON != nil {
		resourcesArg = string(resourcesJSON)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO deployments (id, service_id, image, env_vars, ports, resources, replicas, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		deployment.ID, deployment.ServiceID, deployment.Image,
		string(envVarsJSON), string(portsJSON), resourcesArg,
		deployment.Replicas,
		deployment.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("creating deployment: %w", err)
	}

	// Set this deployment as the active deployment and update service status.
	_, err = tx.ExecContext(ctx,
		`UPDATE services SET active_deployment_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		deployment.ID, deploykit.ServiceStatusDeploying,
		time.Now().UTC().Format(timeFormat), serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating service active deployment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing deployment: %w", err)
	}

	return deployment, nil
}

func (s *DeploymentService) GetDeployment(ctx context.Context, id string) (*deploykit.Deployment, error) {
	d := &deploykit.Deployment{}
	var createdAt string
	var envVarsRaw, portsRaw string
	var resourcesRaw sql.NullString

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, service_id, image, env_vars, ports, resources, replicas, created_at FROM deployments WHERE id = ?`, id,
	).Scan(&d.ID, &d.ServiceID, &d.Image, &envVarsRaw, &portsRaw, &resourcesRaw, &d.Replicas, &createdAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Deployment not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting deployment %s: %w", id, err)
	}

	if err := json.Unmarshal([]byte(envVarsRaw), &d.EnvVars); err != nil {
		return nil, fmt.Errorf("unmarshaling env vars: %w", err)
	}
	if err := json.Unmarshal([]byte(portsRaw), &d.Ports); err != nil {
		return nil, fmt.Errorf("unmarshaling ports: %w", err)
	}
	if resourcesRaw.Valid {
		d.Resources = &deploykit.ResourceLimits{}
		if err := json.Unmarshal([]byte(resourcesRaw.String), d.Resources); err != nil {
			return nil, fmt.Errorf("unmarshaling resources: %w", err)
		}
	}

	d.CreatedAt, _ = time.Parse(timeFormat, createdAt)

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
		`SELECT id, service_id, image, env_vars, ports, resources, replicas, created_at, COUNT(*) OVER() AS total_count
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
		var createdAt string
		var envVarsRaw, portsRaw string
		var resourcesRaw sql.NullString
		if err := rows.Scan(&d.ID, &d.ServiceID, &d.Image, &envVarsRaw, &portsRaw, &resourcesRaw, &d.Replicas, &createdAt, &totalCount); err != nil {
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
		d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		deployments = append(deployments, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating deployment rows: %w", err)
	}

	return deployments, totalCount, nil
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

	// Update the service's active deployment and status.
	now := time.Now().UTC().Format(timeFormat)
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

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

// ServiceService implements deploykit.ServiceService using SQLite.
type ServiceService struct {
	db *DB
}

// NewServiceService creates a new ServiceService backed by the given DB.
func NewServiceService(db *DB) *ServiceService {
	return &ServiceService{db: db}
}

func (s *ServiceService) CreateService(ctx context.Context, projectID string, create deploykit.ServiceCreate) (*deploykit.Service, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	// Verify project exists.
	var exists bool
	err := s.db.db.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE id = ?`, projectID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	} else if err != nil {
		return nil, fmt.Errorf("checking project %s: %w", projectID, err)
	}

	svc := &deploykit.Service{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		Name:      create.Name,
		Status:    deploykit.ServiceStatusCreated,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	_, err = s.db.db.ExecContext(ctx,
		`INSERT INTO services (id, project_id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		svc.ID, svc.ProjectID, svc.Name, svc.Status,
		svc.CreatedAt.Format(timeFormat),
		svc.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, deploykit.Errorf(deploykit.ECONFLICT, "A service with this name already exists in the project.")
		}
		return nil, fmt.Errorf("creating service: %w", err)
	}

	return svc, nil
}

func (s *ServiceService) GetService(ctx context.Context, id string) (*deploykit.Service, error) {
	svc := &deploykit.Service{}
	var createdAt, updatedAt string
	var activeDeploymentID, iconURL, depImage, depCreatedAt sql.NullString
	var depReplicas sql.NullInt64

	err := s.db.db.QueryRowContext(ctx,
		`SELECT s.id, s.project_id, s.name, s.status, s.icon_url, s.active_deployment_id, s.created_at, s.updated_at,
		        d.image, d.replicas, d.created_at
		 FROM services s
		 LEFT JOIN deployments d ON d.id = s.active_deployment_id
		 WHERE s.id = ?`, id,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &iconURL, &activeDeploymentID, &createdAt, &updatedAt,
		&depImage, &depReplicas, &depCreatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting service %s: %w", id, err)
	}

	if activeDeploymentID.Valid {
		svc.ActiveDeploymentID = &activeDeploymentID.String
	}
	if iconURL.Valid {
		svc.IconURL = &iconURL.String
	}
	svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	svc.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
	svc.ActiveDeployment = buildActiveDeployment(svc.ID, activeDeploymentID, depImage, depCreatedAt, depReplicas)

	return svc, nil
}

// buildActiveDeployment assembles a minimal Deployment for list/get hydration —
// only the fields the UI needs (image, replicas, timestamp). Full env/ports/
// resources come from the dedicated deployments endpoint.
func buildActiveDeployment(
	serviceID string,
	id, image, createdAt sql.NullString,
	replicas sql.NullInt64,
) *deploykit.Deployment {
	if !id.Valid || !image.Valid {
		return nil
	}
	dep := &deploykit.Deployment{
		ID:        id.String,
		ServiceID: serviceID,
		Image:     image.String,
		Replicas:  int(replicas.Int64),
	}
	if createdAt.Valid {
		dep.CreatedAt, _ = time.Parse(timeFormat, createdAt.String)
	}
	return dep
}

func (s *ServiceService) ListServices(ctx context.Context, filter deploykit.ServiceFilter) ([]*deploykit.Service, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.ProjectID != nil {
		where = append(where, "s.project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.Name != nil {
		where = append(where, "s.name LIKE ?")
		args = append(args, "%"+*filter.Name+"%")
	}
	if filter.Status != nil {
		where = append(where, "s.status = ?")
		args = append(args, *filter.Status)
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
		`SELECT s.id, s.project_id, s.name, s.status, s.icon_url, s.active_deployment_id, s.created_at, s.updated_at,
		        d.image, d.replicas, d.created_at,
		        COUNT(*) OVER() AS total_count
		 FROM services s
		 LEFT JOIN deployments d ON d.id = s.active_deployment_id
		 WHERE %s ORDER BY s.created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)
	args = append(args, limit, offset)

	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing services: %w", err)
	}
	defer rows.Close()

	var services []*deploykit.Service
	var totalCount int

	for rows.Next() {
		svc := &deploykit.Service{}
		var createdAt, updatedAt string
		var activeDeploymentID, iconURL, depImage, depCreatedAt sql.NullString
		var depReplicas sql.NullInt64
		if err := rows.Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &iconURL, &activeDeploymentID, &createdAt, &updatedAt,
			&depImage, &depReplicas, &depCreatedAt, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scanning service row: %w", err)
		}
		if activeDeploymentID.Valid {
			svc.ActiveDeploymentID = &activeDeploymentID.String
		}
		if iconURL.Valid {
			svc.IconURL = &iconURL.String
		}
		svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		svc.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		svc.ActiveDeployment = buildActiveDeployment(svc.ID, activeDeploymentID, depImage, depCreatedAt, depReplicas)
		services = append(services, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating service rows: %w", err)
	}

	return services, totalCount, nil
}

func (s *ServiceService) UpdateService(ctx context.Context, id string, update deploykit.ServiceUpdate) (*deploykit.Service, error) {
	if err := update.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	svc := &deploykit.Service{}
	var createdAt, updatedAt string
	var activeDeploymentID, iconURL sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT id, project_id, name, status, icon_url, active_deployment_id, created_at, updated_at FROM services WHERE id = ?`, id,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &iconURL, &activeDeploymentID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting service for update %s: %w", id, err)
	}
	if activeDeploymentID.Valid {
		svc.ActiveDeploymentID = &activeDeploymentID.String
	}
	if iconURL.Valid {
		svc.IconURL = &iconURL.String
	}
	svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	sets := []string{"updated_at = ?"}
	svc.UpdatedAt = time.Now().UTC()
	args := []any{svc.UpdatedAt.Format(timeFormat)}

	if update.Name != nil {
		svc.Name = *update.Name
		sets = append(sets, "name = ?")
		args = append(args, svc.Name)
	}
	if update.IconURL != nil {
		if *update.IconURL == "" {
			svc.IconURL = nil
			sets = append(sets, "icon_url = NULL")
		} else {
			v := *update.IconURL
			svc.IconURL = &v
			sets = append(sets, "icon_url = ?")
			args = append(args, v)
		}
	}
	args = append(args, svc.ID)

	_, err = tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE services SET %s WHERE id = ?`, strings.Join(sets, ", ")),
		args...,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, deploykit.Errorf(deploykit.ECONFLICT, "A service with this name already exists in the project.")
		}
		return nil, fmt.Errorf("updating service %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing service update: %w", err)
	}

	return svc, nil
}

func (s *ServiceService) SetServiceStatus(ctx context.Context, id string, status string) error {
	result, err := s.db.db.ExecContext(ctx,
		`UPDATE services SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("updating service status %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	}

	return nil
}

func (s *ServiceService) DeleteService(ctx context.Context, id string) error {
	result, err := s.db.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting service %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	}

	return nil
}

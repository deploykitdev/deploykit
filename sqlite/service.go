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
	var activeDeploymentID sql.NullString

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, status, active_deployment_id, created_at, updated_at FROM services WHERE id = ?`, id,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &activeDeploymentID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting service %s: %w", id, err)
	}

	if activeDeploymentID.Valid {
		svc.ActiveDeploymentID = &activeDeploymentID.String
	}
	svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	svc.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return svc, nil
}

func (s *ServiceService) ListServices(ctx context.Context, filter deploykit.ServiceFilter) ([]*deploykit.Service, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.ProjectID != nil {
		where = append(where, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.Name != nil {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+*filter.Name+"%")
	}
	if filter.Status != nil {
		where = append(where, "status = ?")
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
		`SELECT id, project_id, name, status, active_deployment_id, created_at, updated_at, COUNT(*) OVER() AS total_count
		 FROM services WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
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
		var activeDeploymentID sql.NullString
		if err := rows.Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &activeDeploymentID, &createdAt, &updatedAt, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scanning service row: %w", err)
		}
		if activeDeploymentID.Valid {
			svc.ActiveDeploymentID = &activeDeploymentID.String
		}
		svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		svc.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
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
	var activeDeploymentID sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT id, project_id, name, status, active_deployment_id, created_at, updated_at FROM services WHERE id = ?`, id,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Status, &activeDeploymentID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting service for update %s: %w", id, err)
	}
	if activeDeploymentID.Valid {
		svc.ActiveDeploymentID = &activeDeploymentID.String
	}
	svc.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	if update.Name != nil {
		svc.Name = *update.Name
	}
	svc.UpdatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`UPDATE services SET name = ?, updated_at = ? WHERE id = ?`,
		svc.Name, svc.UpdatedAt.Format(timeFormat), svc.ID,
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

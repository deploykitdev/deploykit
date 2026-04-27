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

// ContainerService implements deploykit.ContainerService using SQLite.
type ContainerService struct {
	db *DB
}

// NewContainerService creates a new ContainerService backed by the given DB.
func NewContainerService(db *DB) *ContainerService {
	return &ContainerService{db: db}
}

func (s *ContainerService) CreateContainer(ctx context.Context, create deploykit.ContainerCreate) (*deploykit.Container, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	status := create.Status
	if status == "" {
		status = deploykit.ContainerStatusCreated
	}

	c := &deploykit.Container{
		ID:                uuid.New().String(),
		ServiceID:         create.ServiceID,
		DeploymentID:      create.DeploymentID,
		DockerContainerID: create.DockerContainerID,
		Status:            status,
		CreatedAt:         time.Now().UTC(),
	}

	_, err := s.db.db.ExecContext(ctx,
		`INSERT INTO containers (id, service_id, deployment_id, docker_container_id, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.ServiceID, c.DeploymentID, c.DockerContainerID, c.Status,
		c.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	return c, nil
}

func (s *ContainerService) GetContainer(ctx context.Context, id string) (*deploykit.Container, error) {
	c := &deploykit.Container{}
	var createdAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, service_id, deployment_id, docker_container_id, status, created_at FROM containers WHERE id = ?`, id,
	).Scan(&c.ID, &c.ServiceID, &c.DeploymentID, &c.DockerContainerID, &c.Status, &createdAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Container not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting container %s: %w", id, err)
	}

	c.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	return c, nil
}

func (s *ContainerService) ListContainers(ctx context.Context, filter deploykit.ContainerFilter) ([]*deploykit.Container, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.ServiceID != nil {
		where = append(where, "service_id = ?")
		args = append(args, *filter.ServiceID)
	}
	if filter.DeploymentID != nil {
		where = append(where, "deployment_id = ?")
		args = append(args, *filter.DeploymentID)
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
		`SELECT id, service_id, deployment_id, docker_container_id, status, created_at, COUNT(*) OVER() AS total_count
		 FROM containers WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)
	args = append(args, limit, offset)

	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing containers: %w", err)
	}
	defer rows.Close()

	var containers []*deploykit.Container
	var totalCount int

	for rows.Next() {
		c := &deploykit.Container{}
		var createdAt string
		if err := rows.Scan(&c.ID, &c.ServiceID, &c.DeploymentID, &c.DockerContainerID, &c.Status, &createdAt, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scanning container row: %w", err)
		}
		c.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		containers = append(containers, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating container rows: %w", err)
	}

	return containers, totalCount, nil
}

func (s *ContainerService) UpdateContainerStatus(ctx context.Context, id string, status string) (*deploykit.Container, error) {
	result, err := s.db.db.ExecContext(ctx,
		`UPDATE containers SET status = ? WHERE id = ?`, status, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating container status %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Container not found.")
	}

	return s.GetContainer(ctx, id)
}

func (s *ContainerService) DeleteContainer(ctx context.Context, id string) error {
	result, err := s.db.db.ExecContext(ctx, `DELETE FROM containers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting container %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Container not found.")
	}

	return nil
}

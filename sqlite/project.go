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

// timeFormat matches SQLite's datetime('now') output format.
const timeFormat = "2006-01-02 15:04:05"

// ProjectService implements deploykit.ProjectService using SQLite.
type ProjectService struct {
	db *DB
}

// NewProjectService creates a new ProjectService backed by the given DB.
func NewProjectService(db *DB) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) CreateProject(ctx context.Context, create deploykit.ProjectCreate) (*deploykit.Project, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	project := &deploykit.Project{
		ID:        uuid.New().String(),
		Name:      create.Name,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Retry slug generation on the unlikely event of a collision.
	const maxAttempts = 3
	for attempt := range maxAttempts {
		project.Slug = deploykit.GenerateSlug(create.Name)

		_, err := s.db.db.ExecContext(ctx,
			`INSERT INTO projects (id, name, slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			project.ID, project.Name, project.Slug,
			project.CreatedAt.Format(timeFormat),
			project.UpdatedAt.Format(timeFormat),
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") && attempt < maxAttempts-1 {
				continue
			}
			return nil, fmt.Errorf("creating project: %w", err)
		}
		return project, nil
	}

	return project, nil
}

func (s *ProjectService) GetProject(ctx context.Context, id string) (*deploykit.Project, error) {
	project := &deploykit.Project{}
	var createdAt, updatedAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, name, slug, created_at, updated_at FROM projects WHERE id = ?`, id,
	).Scan(&project.ID, &project.Name, &project.Slug, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting project %s: %w", id, err)
	}

	project.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	project.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return project, nil
}

func (s *ProjectService) ListProjects(ctx context.Context, filter deploykit.ProjectFilter) ([]*deploykit.Project, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.Name != nil {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+*filter.Name+"%")
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
		`SELECT id, name, slug, created_at, updated_at, COUNT(*) OVER() AS total_count
		 FROM projects WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)
	args = append(args, limit, offset)

	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()

	var projects []*deploykit.Project
	var totalCount int

	for rows.Next() {
		p := &deploykit.Project{}
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &createdAt, &updatedAt, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scanning project row: %w", err)
		}
		p.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		p.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating project rows: %w", err)
	}

	return projects, totalCount, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, id string, update deploykit.ProjectUpdate) (*deploykit.Project, error) {
	if err := update.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	project := &deploykit.Project{}
	var createdAt, updatedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT id, name, slug, created_at, updated_at FROM projects WHERE id = ?`, id,
	).Scan(&project.ID, &project.Name, &project.Slug, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting project for update %s: %w", id, err)
	}
	project.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	if update.Name != nil {
		project.Name = *update.Name
	}
	project.UpdatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`UPDATE projects SET name = ?, updated_at = ? WHERE id = ?`,
		project.Name, project.UpdatedAt.Format(timeFormat), project.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating project %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing project update: %w", err)
	}

	return project, nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var serviceCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM services WHERE project_id = ?`, id,
	).Scan(&serviceCount); err != nil {
		return fmt.Errorf("counting services for project %s: %w", id, err)
	}
	if serviceCount > 0 {
		return deploykit.Errorf(deploykit.ECONFLICT,
			"Project has %d service(s); delete them before deleting the project.", serviceCount)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting project %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Project not found.")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

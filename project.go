package deploykit

import (
	"context"
	"time"
)

// Project represents a deployable application project.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectService manages projects.
type ProjectService interface {
	// CreateProject creates a new project. The ID and timestamps are set
	// by the implementation. On success, the created project is returned.
	CreateProject(ctx context.Context, create ProjectCreate) (*Project, error)

	// GetProject returns a project by ID.
	// Returns ENOTFOUND if the project does not exist.
	GetProject(ctx context.Context, id string) (*Project, error)

	// ListProjects returns a filtered, paginated list of projects
	// and the total matching count.
	ListProjects(ctx context.Context, filter ProjectFilter) ([]*Project, int, error)

	// UpdateProject applies a partial update to a project by ID.
	// Returns the updated project. Returns ENOTFOUND if not found.
	UpdateProject(ctx context.Context, id string, update ProjectUpdate) (*Project, error)

	// DeleteProject permanently removes a project by ID.
	// Returns ENOTFOUND if not found.
	DeleteProject(ctx context.Context, id string) error
}

// ProjectCreate holds fields required to create a project.
type ProjectCreate struct {
	Name string `json:"name"`
}

// Validate checks that all required fields are present.
func (c *ProjectCreate) Validate() error {
	if c.Name == "" {
		return Errorf(EINVALID, "Name is required.")
	}
	return nil
}

// ProjectUpdate holds fields that can be updated on a project.
// Nil pointer fields are left unchanged.
type ProjectUpdate struct {
	Name *string `json:"name"`
}

// Validate checks update fields.
func (u *ProjectUpdate) Validate() error {
	if u.Name != nil && *u.Name == "" {
		return Errorf(EINVALID, "Name cannot be empty.")
	}
	return nil
}

// ProjectFilter controls filtering and pagination for listing projects.
type ProjectFilter struct {
	Name   *string
	Offset int
	Limit  int
}

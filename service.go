package deploykit

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// Service status constants.
const (
	ServiceStatusCreated   = "created"
	ServiceStatusDeploying = "deploying"
	ServiceStatusRunning   = "running"
	ServiceStatusDegraded  = "degraded"
	ServiceStatusFailed    = "failed"
	ServiceStatusStopped   = "stopped"
)

// Service represents a deployable unit within a project.
type Service struct {
	ID                 string      `json:"id"`
	ProjectID          string      `json:"project_id"`
	Name               string      `json:"name"`
	Status             string      `json:"status"`
	IconURL            *string     `json:"icon_url"`
	ActiveDeploymentID *string     `json:"active_deployment_id"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	ActiveDeployment   *Deployment `json:"active_deployment,omitempty"`
}

const maxIconURLLength = 2048

// validateIconURL checks that a supplied icon URL is absolute http/https and
// under the length cap. An empty string is rejected — callers clear the icon
// by sending a JSON null, which hits the nil-pointer branch instead.
func validateIconURL(raw string) error {
	if len(raw) > maxIconURLLength {
		return Errorf(EINVALID, "Icon URL is too long (max %d characters).", maxIconURLLength)
	}
	if strings.TrimSpace(raw) == "" {
		return Errorf(EINVALID, "Icon URL cannot be empty.")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Errorf(EINVALID, "Icon URL is not a valid URL.")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Errorf(EINVALID, "Icon URL must use http or https.")
	}
	if u.Host == "" {
		return Errorf(EINVALID, "Icon URL must include a host.")
	}
	return nil
}

// ServiceService manages services within projects.
type ServiceService interface {
	// CreateService creates a new service in the given project.
	CreateService(ctx context.Context, projectID string, create ServiceCreate) (*Service, error)

	// GetService returns a service by ID.
	// Returns ENOTFOUND if the service does not exist.
	GetService(ctx context.Context, id string) (*Service, error)

	// ListServices returns a filtered, paginated list of services
	// and the total matching count.
	ListServices(ctx context.Context, filter ServiceFilter) ([]*Service, int, error)

	// UpdateService applies a partial update to a service by ID.
	// Returns the updated service. Returns ENOTFOUND if not found.
	UpdateService(ctx context.Context, id string, update ServiceUpdate) (*Service, error)

	// SetServiceStatus updates only the status of a service by ID.
	// Intended for internal lifecycle transitions (reconciler, deployment flow),
	// not user-driven updates. Returns ENOTFOUND if not found.
	SetServiceStatus(ctx context.Context, id string, status string) error

	// DeleteService permanently removes a service by ID.
	// Returns ENOTFOUND if not found.
	DeleteService(ctx context.Context, id string) error
}

// ServiceCreate holds fields required to create a service.
type ServiceCreate struct {
	Name string `json:"name"`
}

// Validate checks that all required fields are present.
func (c *ServiceCreate) Validate() error {
	ve := NewValidationErrors()
	if c.Name == "" {
		ve.Add("name", "Name is required.")
	}
	return ve.Err()
}

// ServiceUpdate holds fields that can be updated on a service.
// Nil pointer fields are left unchanged. To explicitly clear the icon,
// callers send a present IconURL pointer that itself points to an empty
// string — the SQLite layer stores NULL in that case.
type ServiceUpdate struct {
	Name    *string `json:"name"`
	IconURL *string `json:"icon_url"`
}

// Validate checks update fields.
func (u *ServiceUpdate) Validate() error {
	ve := NewValidationErrors()
	if u.Name != nil && *u.Name == "" {
		ve.Add("name", "Name cannot be empty.")
	}
	if u.IconURL != nil && *u.IconURL != "" {
		if err := validateIconURL(*u.IconURL); err != nil {
			ve.Add("icon_url", ErrorMessage(err))
		}
	}
	return ve.Err()
}

// ServiceFilter controls filtering and pagination for listing services.
type ServiceFilter struct {
	ProjectID *string
	Name      *string
	Status    *string
	Offset    int
	Limit     int
}

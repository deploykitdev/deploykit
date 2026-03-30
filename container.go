package deploykit

import (
	"context"
	"time"
)

// Container status constants.
const (
	ContainerStatusCreated = "created"
	ContainerStatusRunning = "running"
	ContainerStatusStopped = "stopped"
	ContainerStatusFailed  = "failed"
)

// Container represents a tracked Docker container instance.
type Container struct {
	ID                string    `json:"id"`
	ServiceID         string    `json:"service_id"`
	DeploymentID      string    `json:"deployment_id"`
	DockerContainerID string    `json:"docker_container_id"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

// ContainerService manages container records.
// Write operations are primarily used by the reconciliation loop.
type ContainerService interface {
	// CreateContainer creates a new container record.
	CreateContainer(ctx context.Context, create ContainerCreate) (*Container, error)

	// GetContainer returns a container by ID.
	// Returns ENOTFOUND if the container does not exist.
	GetContainer(ctx context.Context, id string) (*Container, error)

	// ListContainers returns a filtered, paginated list of containers
	// and the total matching count.
	ListContainers(ctx context.Context, filter ContainerFilter) ([]*Container, int, error)

	// UpdateContainerStatus updates the status of a container.
	// Returns the updated container. Returns ENOTFOUND if not found.
	UpdateContainerStatus(ctx context.Context, id string, status string) (*Container, error)

	// DeleteContainer permanently removes a container record by ID.
	// Returns ENOTFOUND if not found.
	DeleteContainer(ctx context.Context, id string) error
}

// ContainerCreate holds fields required to create a container record.
type ContainerCreate struct {
	ServiceID         string `json:"service_id"`
	DeploymentID      string `json:"deployment_id"`
	DockerContainerID string `json:"docker_container_id"`
	Status            string `json:"status"`
}

// Validate checks that all required fields are present.
func (c *ContainerCreate) Validate() error {
	ve := NewValidationErrors()
	if c.ServiceID == "" {
		ve.Add("service_id", "Service ID is required.")
	}
	if c.DeploymentID == "" {
		ve.Add("deployment_id", "Deployment ID is required.")
	}
	if c.DockerContainerID == "" {
		ve.Add("docker_container_id", "Docker container ID is required.")
	}
	return ve.Err()
}

// ContainerFilter controls filtering and pagination for listing containers.
type ContainerFilter struct {
	ServiceID    *string
	DeploymentID *string
	Status       *string
	Offset       int
	Limit        int
}

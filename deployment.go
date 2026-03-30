package deploykit

import (
	"context"
	"time"
)

// PortMapping defines a port exposed by a container.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"`
	Protocol      string `json:"protocol,omitempty"` // "tcp" (default) or "udp"
}

// ResourceLimits defines CPU and memory constraints for a container.
type ResourceLimits struct {
	CPUShares int `json:"cpu_shares,omitempty"` // Docker CPU shares (relative weight)
	MemoryMB  int `json:"memory_mb,omitempty"`  // Memory limit in megabytes
}

// Deployment represents an immutable snapshot of desired state for a service.
type Deployment struct {
	ID        string            `json:"id"`
	ServiceID string            `json:"service_id"`
	Image     string            `json:"image"`
	EnvVars   map[string]string `json:"env_vars"`
	Ports     []PortMapping     `json:"ports"`
	Resources *ResourceLimits   `json:"resources,omitempty"`
	Replicas  int               `json:"replicas"`
	CreatedAt time.Time         `json:"created_at"`
}

// DeploymentService manages deployments for services.
type DeploymentService interface {
	// CreateDeployment creates a new deployment for the given service
	// and sets it as the active deployment. The service status is set to "deploying".
	CreateDeployment(ctx context.Context, serviceID string, create DeploymentCreate) (*Deployment, error)

	// GetDeployment returns a deployment by ID.
	// Returns ENOTFOUND if the deployment does not exist.
	GetDeployment(ctx context.Context, id string) (*Deployment, error)

	// ListDeployments returns a filtered, paginated list of deployments
	// and the total matching count. Ordered by created_at descending.
	ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*Deployment, int, error)

	// RollbackService sets the active deployment of a service to the given
	// deployment ID. The service status is set to "deploying".
	// Returns the updated service.
	RollbackService(ctx context.Context, serviceID string, deploymentID string) (*Service, error)
}

// DeploymentCreate holds fields required to create a deployment.
type DeploymentCreate struct {
	Image     string            `json:"image"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
	Ports     []PortMapping     `json:"ports,omitempty"`
	Resources *ResourceLimits   `json:"resources,omitempty"`
	Replicas  int               `json:"replicas,omitempty"`
}

// Validate checks that all required fields are present.
func (c *DeploymentCreate) Validate() error {
	ve := NewValidationErrors()
	if c.Image == "" {
		ve.Add("image", "Image is required.")
	}
	if c.Replicas < 0 {
		ve.Add("replicas", "Replicas cannot be negative.")
	}
	return ve.Err()
}

// DeploymentFilter controls filtering and pagination for listing deployments.
type DeploymentFilter struct {
	ServiceID *string
	Offset    int
	Limit     int
}

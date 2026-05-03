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

// Deployment lifecycle status constants.
//
// Lifecycle:
//
//	pending    -> reconciler hasn't picked it up yet
//	starting   -> at least one container created; not yet at desired replica count
//	healthy    -> at desired replica count; this is the deployment serving traffic
//	failed     -> gave up after maxAttempts image pulls or container starts
//	superseded -> was healthy, then a newer deployment took over; containers torn down on next cycle
//	cancelled  -> a newer deployment was created for the same service before this one finished
const (
	DeploymentStatusPending    = "pending"
	DeploymentStatusStarting   = "starting"
	DeploymentStatusHealthy    = "healthy"
	DeploymentStatusFailed     = "failed"
	DeploymentStatusSuperseded = "superseded"
	DeploymentStatusCancelled  = "cancelled"
)

// Deployment represents an immutable snapshot of desired state for a service.
type Deployment struct {
	ID            string            `json:"id"`
	ServiceID     string            `json:"service_id"`
	Image         string            `json:"image"`
	EnvVars       map[string]string `json:"env_vars"`
	Ports         []PortMapping     `json:"ports"`
	Resources     *ResourceLimits   `json:"resources,omitempty"`
	Replicas      int               `json:"replicas"`
	Status        string            `json:"status"`
	FailureReason *string           `json:"failure_reason,omitempty"`
	ExitCode      *int              `json:"exit_code,omitempty"`
	LogTail       *string           `json:"log_tail,omitempty"`
	AttemptCount  int               `json:"attempt_count"`
	// BaselineRestartCount is the Docker RestartCount observed at promotion
	// time. The reconciler uses it to distinguish a healthy deployment that
	// has accumulated legitimate restarts (daemon hiccups, etc.) from one
	// that is actively crashlooping after promotion.
	BaselineRestartCount int        `json:"-"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	HealthyAt            *time.Time `json:"healthy_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// DeploymentService manages deployments for services.
type DeploymentService interface {
	// CreateDeployment inserts a new deployment for the given service with
	// status "pending". The service's active_deployment_id is NOT touched —
	// the reconciler flips it once the new deployment becomes healthy. Any
	// prior pending/starting deployment for the same service is marked
	// "cancelled" in the same transaction.
	//
	// If the service has no active deployment yet, service status is set to
	// "deploying" so the first-deploy UX still shows progress.
	CreateDeployment(ctx context.Context, serviceID string, create DeploymentCreate) (*Deployment, error)

	// GetDeployment returns a deployment by ID.
	// Returns ENOTFOUND if the deployment does not exist.
	GetDeployment(ctx context.Context, id string) (*Deployment, error)

	// ListDeployments returns a filtered, paginated list of deployments
	// and the total matching count. Ordered by created_at descending.
	ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*Deployment, int, error)

	// ListInFlightDeployments returns all deployments with status pending,
	// starting, or healthy. The reconciler uses this as its desired-state
	// source so superseded/failed/cancelled deployments are excluded.
	ListInFlightDeployments(ctx context.Context) ([]*Deployment, error)

	// MarkDeploymentStarting transitions a pending deployment to starting and
	// stamps started_at. No-op if already starting.
	MarkDeploymentStarting(ctx context.Context, id string) error

	// MarkDeploymentHealthy transitions a starting deployment to healthy,
	// stamps healthy_at, persists baselineRestartCount (the Docker RestartCount
	// observed at promotion), supersedes the previous healthy deployment for the
	// same service (if any), and flips services.active_deployment_id atomically.
	// Returns the prior active deployment ID (or "" if none) so the caller can
	// publish supersede events.
	MarkDeploymentHealthy(ctx context.Context, id string, baselineRestartCount int) (priorActiveID string, err error)

	// MarkDeploymentFailed transitions a deployment to failed with the given
	// reason. exitCode is nil when the container never started (e.g. image
	// pull failure). logTail must be ≤10 KB; callers truncate before passing.
	// If the service has no other healthy deployment, the service status is
	// also set to "failed".
	MarkDeploymentFailed(ctx context.Context, id string, reason string, exitCode *int, logTail string) error

	// IncrementDeploymentAttempt bumps attempt_count and returns the new value.
	IncrementDeploymentAttempt(ctx context.Context, id string) (int, error)

	// RollbackService sets the active deployment of a service to the given
	// deployment ID, marks it healthy, and supersedes the previously-active
	// deployment. Returns the updated service.
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

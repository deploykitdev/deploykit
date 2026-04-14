package deploykit

import "context"

// NetworkPrefix is prepended to project slugs when creating Docker networks.
const NetworkPrefix = "dk-"

// Container label keys applied to every DeployKit-managed container so the
// reconciler can rediscover them from the container runtime.
const (
	LabelManagedBy    = "managed-by"
	LabelManagedValue = "deploykit"
	LabelProjectID    = "deploykit.project-id"
	LabelServiceID    = "deploykit.service-id"
	LabelDeploymentID = "deploykit.deployment-id"
	LabelReplicaIndex = "deploykit.replica-index"
)

// ContainerSpec describes a single container the reconciler wants to run.
type ContainerSpec struct {
	Name        string
	Image       string
	NetworkName string
	EnvVars     map[string]string
	Ports       []PortMapping
	Resources   *ResourceLimits
	Labels      map[string]string
}

// RunningContainer describes a container currently known to the runtime.
// Fields parsed from labels may be empty if the labels are missing/malformed.
type RunningContainer struct {
	DockerID     string
	Name         string
	ProjectID    string
	ServiceID    string
	DeploymentID string
	ReplicaIndex int
	State        string
}

// Provisioner manages infrastructure resources (networks, containers, images)
// backing DeployKit projects.
type Provisioner interface {
	// EnsureNetwork creates a network for the project if it does not already
	// exist. The network is named using NetworkPrefix + project slug.
	// Calling it when the network already exists is a no-op.
	EnsureNetwork(ctx context.Context, project *Project) error

	// RemoveNetwork removes a DeployKit-managed network by its full name
	// (e.g., "dk-my-app-a1b2c3"). Removing a non-existent network is a no-op.
	RemoveNetwork(ctx context.Context, networkName string) error

	// ListNetworks returns the names of all networks whose names begin with
	// NetworkPrefix.
	ListNetworks(ctx context.Context) ([]string, error)

	// EnsureImage pulls the image if it is not already present locally.
	EnsureImage(ctx context.Context, image string) error

	// CreateAndStartContainer creates a container from the spec and starts it.
	// Returns the runtime container ID.
	CreateAndStartContainer(ctx context.Context, spec ContainerSpec) (string, error)

	// StopAndRemoveContainer stops and removes a container by its runtime ID.
	// Removing a non-existent container is a no-op.
	StopAndRemoveContainer(ctx context.Context, dockerID string) error

	// ListContainers returns all containers labelled as managed by DeployKit.
	ListContainers(ctx context.Context) ([]RunningContainer, error)
}

// NetworkName returns the DeployKit network name for a project.
func NetworkName(project *Project) string {
	return NetworkPrefix + project.Slug
}

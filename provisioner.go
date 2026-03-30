package deploykit

import "context"

// NetworkPrefix is prepended to project slugs when creating Docker networks.
const NetworkPrefix = "dk-"

// Provisioner manages infrastructure resources (networks, containers, etc.)
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
}

// NetworkName returns the DeployKit network name for a project.
func NetworkName(project *Project) string {
	return NetworkPrefix + project.Slug
}

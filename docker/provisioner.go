package docker

import (
	"context"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/heyjorgedev/deploykit"
)

// managedByLabel is applied to all networks created by DeployKit.
const managedByLabel = "managed-by=deploykit"

// EnsureNetwork creates a bridge network for the project if it does not
// already exist.
func (c *Client) EnsureNetwork(ctx context.Context, project *deploykit.Project) error {
	name := deploykit.NetworkName(project)

	existing, err := c.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", managedByLabel),
			filters.Arg("name", name),
		),
	})
	if err != nil {
		return fmt.Errorf("listing networks for %q: %w", name, err)
	}
	for _, n := range existing {
		if n.Name == name {
			c.logger.Debug("network already exists", "network", name)
			return nil
		}
	}

	_, err = c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"managed-by": "deploykit",
		},
	})
	if err != nil {
		return fmt.Errorf("creating network %q: %w", name, err)
	}

	c.logger.Info("network created", "network", name)
	return nil
}

// RemoveNetwork removes a DeployKit-managed network by name. It is a no-op
// if the network does not exist.
func (c *Client) RemoveNetwork(ctx context.Context, networkName string) error {
	err := c.cli.NetworkRemove(ctx, networkName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.logger.Debug("network already removed", "network", networkName)
			return nil
		}
		return fmt.Errorf("removing network %q: %w", networkName, err)
	}

	c.logger.Info("network removed", "network", networkName)
	return nil
}

// ListNetworks returns the names of all Docker networks managed by DeployKit.
func (c *Client) ListNetworks(ctx context.Context) ([]string, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", managedByLabel)),
	})
	if err != nil {
		return nil, fmt.Errorf("listing deploykit networks: %w", err)
	}

	var names []string
	for _, n := range networks {
		names = append(names, n.Name)
	}
	return names, nil
}

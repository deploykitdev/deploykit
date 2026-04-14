package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/heyjorgedev/deploykit"
)

// EnsureImage pulls the image if it is not already present locally.
func (c *Client) EnsureImage(ctx context.Context, ref string) error {
	imgs, err := c.cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", ref)),
	})
	if err != nil {
		return fmt.Errorf("listing images for %q: %w", ref, err)
	}
	if len(imgs) > 0 {
		return nil
	}

	c.logger.Info("pulling image", "image", ref)
	rc, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", ref, err)
	}
	defer rc.Close()

	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("draining pull stream for %q: %w", ref, err)
	}
	c.logger.Info("image pulled", "image", ref)
	return nil
}

// CreateAndStartContainer creates a container from the spec and starts it.
func (c *Client) CreateAndStartContainer(ctx context.Context, spec deploykit.ContainerSpec) (string, error) {
	env := make([]string, 0, len(spec.EnvVars))
	for k, v := range spec.EnvVars {
		env = append(env, k+"="+v)
	}

	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	for _, p := range spec.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		port, err := nat.NewPort(proto, strconv.Itoa(p.ContainerPort))
		if err != nil {
			return "", fmt.Errorf("invalid port %d/%s: %w", p.ContainerPort, proto, err)
		}
		exposed[port] = struct{}{}
		if p.HostPort > 0 {
			bindings[port] = []nat.PortBinding{{HostPort: strconv.Itoa(p.HostPort)}}
		}
	}

	cfg := &container.Config{
		Image:        spec.Image,
		Env:          env,
		ExposedPorts: exposed,
		Labels:       spec.Labels,
	}

	hostCfg := &container.HostConfig{
		PortBindings: bindings,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}
	if spec.Resources != nil {
		if spec.Resources.MemoryMB > 0 {
			hostCfg.Memory = int64(spec.Resources.MemoryMB) * 1024 * 1024
		}
		if spec.Resources.CPUShares > 0 {
			hostCfg.CPUShares = int64(spec.Resources.CPUShares)
		}
	}

	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			spec.NetworkName: {},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("creating container %q: %w", spec.Name, err)
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Best-effort cleanup so we don't leak a created-but-not-started container.
		_ = c.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("starting container %q: %w", spec.Name, err)
	}

	c.logger.Info("container started", "name", spec.Name, "id", resp.ID, "image", spec.Image)
	return resp.ID, nil
}

// StopAndRemoveContainer stops and removes a container by runtime ID.
func (c *Client) StopAndRemoveContainer(ctx context.Context, dockerID string) error {
	timeout := 10
	if err := c.cli.ContainerStop(ctx, dockerID, container.StopOptions{Timeout: &timeout}); err != nil {
		if !errdefs.IsNotFound(err) {
			c.logger.Warn("stopping container failed, will force remove", "id", dockerID, "err", err)
		}
	}

	err := c.cli.ContainerRemove(ctx, dockerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("removing container %s: %w", dockerID, err)
	}

	c.logger.Info("container removed", "id", dockerID)
	return nil
}

// ListContainers returns all containers labelled as managed by DeployKit.
func (c *Client) ListContainers(ctx context.Context) ([]deploykit.RunningContainer, error) {
	cs, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", deploykit.LabelManagedBy+"="+deploykit.LabelManagedValue),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("listing deploykit containers: %w", err)
	}

	out := make([]deploykit.RunningContainer, 0, len(cs))
	for _, ct := range cs {
		rc := deploykit.RunningContainer{
			DockerID:     ct.ID,
			ProjectID:    ct.Labels[deploykit.LabelProjectID],
			ServiceID:    ct.Labels[deploykit.LabelServiceID],
			DeploymentID: ct.Labels[deploykit.LabelDeploymentID],
			State:        ct.State,
		}
		if len(ct.Names) > 0 {
			rc.Name = strings.TrimPrefix(ct.Names[0], "/")
		}
		if s := ct.Labels[deploykit.LabelReplicaIndex]; s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				rc.ReplicaIndex = n
			}
		}
		out = append(out, rc)
	}
	return out, nil
}

package docker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker/client"
)

// Client wraps the Docker API client and manages its lifecycle.
type Client struct {
	cli    *client.Client
	logger *slog.Logger
}

// NewClient creates a new Docker client with the given logger.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		logger: logger,
	}
}

// Open initializes the Docker API client using environment configuration
// (DOCKER_HOST, DOCKER_TLS_VERIFY, etc.) and negotiates the API version.
func (c *Client) Open() error {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}

	c.cli = cli
	c.logger.Info("docker client initialized")
	return nil
}

// Ping verifies that the Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("pinging docker daemon: %w", err)
	}

	c.logger.Info("docker daemon connected", "api_version", resp.APIVersion)
	return nil
}

// Close closes the underlying Docker API client.
func (c *Client) Close() error {
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}

package deploykit

import (
	"context"
	"regexp"
	"time"
)

// EnvVarScope identifies what an EnvVar is attached to.
type EnvVarScope string

const (
	// EnvVarScopeProject denotes an env var shared by all services in a project.
	EnvVarScopeProject EnvVarScope = "project"
	// EnvVarScopeService denotes an env var that overrides project-level values
	// for a single service.
	EnvVarScopeService EnvVarScope = "service"
)

// Valid reports whether the scope is a known value.
func (s EnvVarScope) Valid() bool {
	return s == EnvVarScopeProject || s == EnvVarScopeService
}

// envVarKeyPattern enforces POSIX-ish environment variable naming:
// letters, digits and underscores, not starting with a digit.
var envVarKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// EnvVar represents an environment variable attached to a project or service.
type EnvVar struct {
	ID        string      `json:"id"`
	Scope     EnvVarScope `json:"scope"`
	ScopeID   string      `json:"scope_id"`
	Key       string      `json:"key"`
	Value     string      `json:"value"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// EnvVarService manages environment variables for projects and services.
type EnvVarService interface {
	// CreateEnvVar creates a new env var at the given scope.
	// Returns ECONFLICT if (scope, scope_id, key) already exists,
	// ENOTFOUND if the scope target does not exist.
	CreateEnvVar(ctx context.Context, scope EnvVarScope, scopeID string, create EnvVarCreate) (*EnvVar, error)

	// GetEnvVar returns an env var by ID.
	// Returns ENOTFOUND if it does not exist.
	GetEnvVar(ctx context.Context, id string) (*EnvVar, error)

	// ListEnvVars returns all env vars for the given scope, ordered by key.
	ListEnvVars(ctx context.Context, scope EnvVarScope, scopeID string) ([]*EnvVar, error)

	// UpdateEnvVar applies a partial update to an env var by ID.
	// Returns the updated env var. Returns ENOTFOUND if not found.
	UpdateEnvVar(ctx context.Context, id string, update EnvVarUpdate) (*EnvVar, error)

	// DeleteEnvVar permanently removes an env var by ID.
	// Returns ENOTFOUND if not found.
	DeleteEnvVar(ctx context.Context, id string) error

	// ResolveForService returns the merged env var set for a service:
	// project-level vars overlaid by service-level vars (service wins).
	// Returns an empty non-nil map if no vars are defined.
	// Returns ENOTFOUND if the service does not exist.
	ResolveForService(ctx context.Context, serviceID string) (map[string]string, error)
}

// EnvVarCreate holds fields required to create an env var.
type EnvVarCreate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Validate checks that all required fields are present and well-formed.
func (c *EnvVarCreate) Validate() error {
	ve := NewValidationErrors()
	if c.Key == "" {
		ve.Add("key", "Key is required.")
	} else if !envVarKeyPattern.MatchString(c.Key) {
		ve.Add("key", "Key must contain only letters, digits and underscores, and cannot start with a digit.")
	}
	return ve.Err()
}

// EnvVarUpdate holds fields that can be updated on an env var.
// Nil pointer fields are left unchanged.
type EnvVarUpdate struct {
	Value *string `json:"value"`
}

// Validate checks update fields.
func (u *EnvVarUpdate) Validate() error {
	return nil
}

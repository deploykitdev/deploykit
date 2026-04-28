package deploykit

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// EnvVarScope identifies what an EnvVar is attached to.
type EnvVarScope string

const (
	// EnvVarScopeProject denotes an env var shared by all services in a project.
	EnvVarScopeProject EnvVarScope = "project"
	// EnvVarScopeGroup denotes an env var attached to a canvas group node and
	// inherited by every service whose canvas node sits inside that group.
	// Sits between project and service in the resolution chain.
	EnvVarScopeGroup EnvVarScope = "group"
	// EnvVarScopeService denotes an env var that overrides project- and
	// group-level values for a single service.
	EnvVarScopeService EnvVarScope = "service"
)

// Valid reports whether the scope is a known value.
func (s EnvVarScope) Valid() bool {
	return s == EnvVarScopeProject || s == EnvVarScopeGroup || s == EnvVarScopeService
}

// envVarKeyPattern enforces POSIX-ish environment variable naming:
// letters, digits and underscores, not starting with a digit.
var envVarKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envVarRefPattern matches `${{service-name.HOST}}` placeholders inside an env
// var value. Service names follow the same shape as Docker-safe slugs: lower
// or upper case letters, digits, and hyphens. The double-brace form was chosen
// so users can still use shell-style `${VAR}` substitutions in their values
// without collision.
var envVarRefPattern = regexp.MustCompile(`\$\{\{\s*([A-Za-z0-9][A-Za-z0-9_-]*)\.HOST\s*\}\}`)

// ResolveServiceRefs walks `value` for `${{name.HOST}}` placeholders and
// substitutes each one with `lookup(name)`. If `lookup` returns ok=false for a
// given name, the original placeholder text is left intact. The returned refs
// slice contains the unique referenced service names (in first-seen order),
// regardless of whether they resolved.
func ResolveServiceRefs(value string, lookup func(name string) (hostname string, ok bool)) (string, []string) {
	if !strings.Contains(value, "${{") {
		return value, nil
	}
	seen := make(map[string]bool)
	var refs []string
	resolved := envVarRefPattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := envVarRefPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		name := groups[1]
		if !seen[name] {
			seen[name] = true
			refs = append(refs, name)
		}
		host, ok := lookup(name)
		if !ok {
			return match
		}
		return host
	})
	return resolved, refs
}

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
	// Any `${{name.HOST}}` placeholders are resolved to the Docker hostname
	// of the referenced service (`dk-{project_slug}-{service_name}-0`); if the
	// referenced service does not exist, the placeholder is left intact.
	// Returns an empty non-nil map if no vars are defined.
	// Returns ENOTFOUND if the service does not exist.
	ResolveForService(ctx context.Context, serviceID string) (map[string]string, error)

	// ResolveForServiceWithRefs is ResolveForService that additionally returns
	// the service references found in each env var value, keyed by env var key.
	// Used by callers that need to sync canvas auto-edges in lockstep with
	// deployment snapshots.
	ResolveForServiceWithRefs(ctx context.Context, serviceID string) (map[string]string, map[string][]string, error)
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

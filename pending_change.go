package deploykit

import (
	"context"
	"encoding/json"
	"time"
)

// PendingChangeOp identifies the kind of operation a pending change represents.
type PendingChangeOp string

const (
	PendingOpProjectUpdate PendingChangeOp = "project.update"
	PendingOpServiceCreate PendingChangeOp = "service.create"
	PendingOpServiceUpdate PendingChangeOp = "service.update"
	PendingOpServiceDelete PendingChangeOp = "service.delete"
	PendingOpEnvVarCreate  PendingChangeOp = "env_var.create"
	PendingOpEnvVarUpdate  PendingChangeOp = "env_var.update"
	PendingOpEnvVarDelete  PendingChangeOp = "env_var.delete"
)

// PendingChangeTarget identifies what a pending change acts on.
type PendingChangeTarget string

const (
	PendingTargetProject PendingChangeTarget = "project"
	PendingTargetService PendingChangeTarget = "service"
	PendingTargetEnvVar  PendingChangeTarget = "env_var"
)

// PendingChange is one entry in the project's changelog. Entries are appended
// as users edit, kept ordered by Seq within a project, and cleared atomically
// when Apply runs.
type PendingChange struct {
	ID           string              `json:"id"`
	ProjectID    string              `json:"project_id"`
	Seq          int64               `json:"seq"`
	Op           PendingChangeOp     `json:"op"`
	TargetType   PendingChangeTarget `json:"target_type"`
	TargetID     *string             `json:"target_id,omitempty"`
	TargetTempID *string             `json:"target_temp_id,omitempty"`
	ParentTempID *string             `json:"parent_temp_id,omitempty"`
	Payload      json.RawMessage     `json:"payload"`
	UserID       *string             `json:"user_id,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
}

// PendingChangeInput is the write shape for appending a new entry. The service
// fills in ID, Seq, and CreatedAt.
type PendingChangeInput struct {
	Op           PendingChangeOp     `json:"op"`
	TargetType   PendingChangeTarget `json:"target_type"`
	TargetID     *string             `json:"target_id,omitempty"`
	TargetTempID *string             `json:"target_temp_id,omitempty"`
	ParentTempID *string             `json:"parent_temp_id,omitempty"`
	Payload      json.RawMessage     `json:"payload"`
	UserID       *string             `json:"user_id,omitempty"`
}

// ApplyResult is returned from Apply and reports what happened.
type ApplyResult struct {
	// AppliedCount is the number of entries that were applied.
	AppliedCount int `json:"applied_count"`
	// TempIDToServiceID maps each service.create entry's temp id to the
	// real service ID it resolved to.
	TempIDToServiceID map[string]string `json:"temp_id_to_service_id"`
	// RedeployedServiceIDs are services (not newly created by this apply)
	// whose active deployment was refreshed because their env vars changed.
	RedeployedServiceIDs []string `json:"redeployed_service_ids"`
	// CreatedDeployments are the new deployments created during the apply
	// (both initial deploys from service.create and refreshes from env var
	// changes). Callers use these to publish EventDeploymentCreated events.
	CreatedDeployments []*Deployment `json:"created_deployments"`
}

// PendingChangeService manages the append-only changelog for a project.
type PendingChangeService interface {
	// List returns the project's pending changes ordered by Seq ascending.
	// Returns an empty slice if there are none.
	List(ctx context.Context, projectID string) ([]*PendingChange, error)

	// Append records a new change entry at the next sequence number.
	Append(ctx context.Context, projectID string, input PendingChangeInput) (*PendingChange, error)

	// DiscardAll removes every pending change for the project without
	// touching any applied state.
	DiscardAll(ctx context.Context, projectID string) error

	// RemoveByTempID removes every pending change that references the given
	// temp id either as target_temp_id or parent_temp_id. Returns the IDs of
	// the removed rows so callers can broadcast a targeted cache update.
	// Used to discard the entries tied to a pending-added service when the
	// user deletes its canvas node before deploy.
	RemoveByTempID(ctx context.Context, projectID string, tempID string) ([]string, error)

	// RemoveByID removes a single pending change by ID. Returns ENOTFOUND if
	// the ID doesn't exist under the given project.
	RemoveByID(ctx context.Context, projectID string, changeID string) error

	// CoalesceServiceReparent atomically replaces any prior staged reparent
	// entries for `serviceID` with at most one new entry that records the move
	// from the truly-applied parent to `newParentID`. If the resulting net
	// change is zero (service is back where it was last deployed), no new
	// entry is created.
	//
	// `appliedParentIfNoPrior` is the value the function should treat as the
	// applied state ONLY when no prior staged reparent exists. If a prior
	// entry is found, its PreviousParentID wins instead (it captured the
	// truly-applied state at the moment it was staged). Callers typically
	// pass the canvas_nodes row's parent_id from before this upsert.
	//
	// Returns the IDs of any prior entries that were dropped and the newly
	// appended entry (nil if net-zero). The whole operation runs in one
	// transaction so concurrent callers can't produce duplicate entries.
	CoalesceServiceReparent(
		ctx context.Context,
		projectID string,
		serviceID string,
		appliedParentIfNoPrior string,
		newParentID string,
		userID *string,
	) (removedIDs []string, appended *PendingChange, err error)

	// Apply walks the project's pending changes in sequence order, mutates
	// real state inside a single transaction, clears the log on success, and
	// returns a summary. On any error the transaction is rolled back and the
	// log is left intact.
	Apply(ctx context.Context, projectID string) (*ApplyResult, error)
}

// --- Op-specific payload shapes ---

// ProjectUpdatePayload is the payload for a PendingOpProjectUpdate entry.
type ProjectUpdatePayload struct {
	Name *string `json:"name,omitempty"`
}

// ServiceCreatePayload is the payload for a PendingOpServiceCreate entry.
// The canvas node ID of the pending-added service serves as the TargetTempID.
type ServiceCreatePayload struct {
	Name    string                 `json:"name"`
	Image   string                 `json:"image"`
	IconURL *string                `json:"icon_url,omitempty"`
	Ports   []PortMapping          `json:"ports,omitempty"`
	EnvVars []EnvVarCreatePayload `json:"env_vars,omitempty"`
}

// ServiceUpdatePayload is the payload for a PendingOpServiceUpdate entry.
type ServiceUpdatePayload struct {
	Name    *string `json:"name,omitempty"`
	IconURL *string `json:"icon_url,omitempty"`
	// Reparented marks an update that didn't change any fields on the service
	// itself but moved its canvas node into or out of a group. The apply path
	// uses this to refresh the deployment snapshot so the service picks up
	// (or drops) the new group's env vars.
	Reparented bool `json:"reparented,omitempty"`
	// PreviousParentID captures the canvas node's parent_id at the moment the
	// FIRST reparent was staged (i.e. the applied state). Used to coalesce
	// repeated drag-in/drag-out cycles into a single pending entry — and to
	// drop the entry entirely when the user moves the service back to where
	// it started. Empty string means "no parent" (top-level). Only meaningful
	// when Reparented is true.
	PreviousParentID string `json:"previous_parent_id,omitempty"`
}

// EnvVarCreatePayload is the payload for a PendingOpEnvVarCreate entry.
// When ParentTempID is set on the parent entry, ScopeID is ignored and the
// env var is attached to the freshly created service at apply time.
type EnvVarCreatePayload struct {
	Scope EnvVarScope `json:"scope"`
	Key   string      `json:"key"`
	Value string      `json:"value"`
}

// EnvVarUpdatePayload is the payload for a PendingOpEnvVarUpdate entry.
// Scope/ScopeID/Key + OldValue are denormalized from the env_vars row at
// stage time so the compacted diff view can route, label, and show
// before→after without a lookup.
type EnvVarUpdatePayload struct {
	Value    string      `json:"value"`
	OldValue string      `json:"old_value"`
	Scope    EnvVarScope `json:"scope"`
	ScopeID  string      `json:"scope_id"`
	Key      string      `json:"key"`
}

// EnvVarDeletePayload is the payload for a PendingOpEnvVarDelete entry. Same
// denormalization rationale as EnvVarUpdatePayload.
type EnvVarDeletePayload struct {
	Scope    EnvVarScope `json:"scope"`
	ScopeID  string      `json:"scope_id"`
	Key      string      `json:"key"`
	OldValue string      `json:"old_value"`
}

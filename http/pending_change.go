package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/heyjorgedev/deploykit"
)

// handleListPendingChanges returns the project's pending change log.
func (s *Server) handleListPendingChanges(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	changes, err := s.PendingChangeService.List(r.Context(), projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"data": changes})
}

// handleDiscardPendingChanges clears every pending change for the project
// without touching applied state.
func (s *Server) handleDiscardPendingChanges(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.PendingChangeService.DiscardAll(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Pending-created services' canvas nodes are removed by DiscardAll, so
	// push the refreshed canvas state before announcing the clear.
	s.broadcastCanvasState(r.Context(), projectID)
	s.canvasHub.broadcastPendingCleared(projectID)

	w.WriteHeader(http.StatusNoContent)
}

// handleDeployProject applies all pending changes atomically, publishes
// deployment events, and triggers the reconciler once.
func (s *Server) handleDeployProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	res, err := s.PendingChangeService.Apply(r.Context(), projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Publish an event per created or refreshed deployment so the canvas hub
	// forwards deployment:created to every connected client.
	if s.EventBus != nil {
		for _, dep := range res.CreatedDeployments {
			s.EventBus.Publish(r.Context(), deploykit.Event{
				Type:      deploykit.EventDeploymentCreated,
				ProjectID: projectID,
				Payload:   deploykit.DeploymentCreatedPayload{Deployment: dep},
			})
		}
	}

	// Canvas state may have changed (new service_ids on nodes, deleted nodes).
	// Push the fresh state to all clients in the room before the
	// pending-changes:applied notice so they can re-hydrate in the right order.
	s.broadcastCanvasState(r.Context(), projectID)
	s.canvasHub.broadcastPendingApplied(projectID, res)

	if res.AppliedCount > 0 && s.Reconciler != nil {
		s.Reconciler.Trigger()
	}

	jsonResponse(w, http.StatusOK, res)
}

// handleCreatePendingServiceEnvVar stages an env var under a pending-added
// service — the one referenced by tempId in its service.create entry.
// Uses parent_temp_id so Apply attaches the env var to the real service ID
// once the service is created in the same transaction.
func (s *Server) handleCreatePendingServiceEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	tempID := r.PathValue("tempId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Verify a matching pending service.create exists + collect existing env
	// var keys under it so we can reject duplicates up front rather than at
	// deploy time.
	changes, err := s.PendingChangeService.List(r.Context(), projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	var found bool
	existingKeys := map[string]bool{}
	for _, c := range changes {
		if c.Op == deploykit.PendingOpServiceCreate && c.TargetTempID != nil && *c.TargetTempID == tempID {
			found = true
			var p deploykit.ServiceCreatePayload
			if json.Unmarshal(c.Payload, &p) == nil {
				for _, ev := range p.EnvVars {
					existingKeys[ev.Key] = true
				}
			}
		}
		if c.Op == deploykit.PendingOpEnvVarCreate && c.ParentTempID != nil && *c.ParentTempID == tempID {
			var p deploykit.EnvVarCreatePayload
			if json.Unmarshal(c.Payload, &p) == nil {
				existingKeys[p.Key] = true
			}
		}
	}
	if !found {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Pending service not found."))
		return
	}

	var req deploykit.EnvVarCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if err := req.Validate(); err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existingKeys[req.Key] {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ECONFLICT, "Env var %q already staged on this service.", req.Key))
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeService,
		Key:   req.Key,
		Value: req.Value,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpEnvVarCreate,
		TargetType:   deploykit.PendingTargetEnvVar,
		ParentTempID: &tempID,
		Payload:      payload,
		UserID:       currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusCreated, pc)
}

// handleDeletePendingChange removes a single pending change entry by ID.
// Used by the pending-service panel to back individual env var changes out
// before deploy. Only valid for env_var.* ops — removing a service.create
// should go through node:delete on the canvas so the node is cleaned up too.
func (s *Server) handleDeletePendingChange(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	changeID := r.PathValue("changeId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Find the change first so we can guard ops that need side-channel cleanup.
	changes, err := s.PendingChangeService.List(r.Context(), projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	var target *deploykit.PendingChange
	for _, c := range changes {
		if c.ID == changeID {
			target = c
			break
		}
	}
	if target == nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Pending change not found."))
		return
	}
	switch {
	case target.Op == deploykit.PendingOpServiceCreate:
		// Canvas-node lifecycle owns the service.create — removing just the
		// log entry would orphan the visual placeholder.
		s.errorResponse(w, r, deploykit.Errorf(
			deploykit.EINVALID,
			"Delete the service's canvas node to cancel its creation.",
		))
		return
	case target.Op == deploykit.PendingOpServiceDelete:
		s.errorResponse(w, r, deploykit.Errorf(
			deploykit.EINVALID,
			"Discard all pending changes to cancel this service deletion.",
		))
		return
	case strings.HasPrefix(string(target.Op), "env_var."),
		target.Op == deploykit.PendingOpProjectUpdate,
		target.Op == deploykit.PendingOpServiceUpdate:
		// OK — safe to remove as a standalone entry.
	default:
		s.errorResponse(w, r, deploykit.Errorf(
			deploykit.EINVALID,
			"This change can't be removed individually; discard all pending changes instead.",
		))
		return
	}

	if err := s.PendingChangeService.RemoveByID(r.Context(), projectID, changeID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangesRemoved(projectID, []string{changeID})
	w.WriteHeader(http.StatusNoContent)
}

// broadcastCanvasState sends the current canvas state to every client in the
// project's room. Used after apply, when multiple nodes/services may have
// changed and individual deltas would be noisier than a single refresh.
func (s *Server) broadcastCanvasState(ctx context.Context, projectID string) {
	nodes, edges, err := s.CanvasService.GetCanvasState(ctx, projectID)
	if err != nil {
		s.logger.Error("loading canvas state for broadcast", "err", err, "project_id", projectID)
		return
	}
	if nodes == nil {
		nodes = []*deploykit.CanvasNode{}
	}
	if edges == nil {
		edges = []*deploykit.CanvasEdge{}
	}
	payload, err := json.Marshal(map[string]any{"nodes": nodes, "edges": edges})
	if err != nil {
		return
	}
	msg, err := json.Marshal(wsMessage{Type: "canvas:state", Payload: payload})
	if err != nil {
		return
	}
	s.canvasHub.broadcastToProject(projectID, msg)
}

package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/heyjorgedev/deploykit"
)

// --- Project-scoped handlers ---
//
// All env var mutations stage a pending change instead of writing to the
// env_vars table directly. Applied state mutates only on /projects/:id/deploy.

func (s *Server) handleCreateProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
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
	if err := s.checkEnvVarKeyFree(r.Context(), projectID, deploykit.EnvVarScopeProject, projectID, req.Key); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarCreatePayload{
		Scope: deploykit.EnvVarScopeProject,
		Key:   req.Key,
		Value: req.Value,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarCreate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &projectID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusCreated, pc)
}

func (s *Server) handleListProjectEnvVars(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), projectID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	envVars, err := s.EnvVarService.ListEnvVars(r.Context(), deploykit.EnvVarScopeProject, projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"data": envVars})
}

func (s *Server) handleUpdateProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	envVarID := r.PathValue("envVarId")

	existing, err := s.EnvVarService.GetEnvVar(r.Context(), envVarID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.Scope != deploykit.EnvVarScopeProject || existing.ScopeID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found."))
		return
	}

	var req deploykit.EnvVarUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if req.Value == nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "value is required."))
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarUpdatePayload{
		Value:    *req.Value,
		OldValue: existing.Value,
		Scope:    existing.Scope,
		ScopeID:  existing.ScopeID,
		Key:      existing.Key,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarUpdate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusOK, pc)
}

func (s *Server) handleDeleteProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	envVarID := r.PathValue("envVarId")

	existing, err := s.EnvVarService.GetEnvVar(r.Context(), envVarID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.Scope != deploykit.EnvVarScopeProject || existing.ScopeID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found."))
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarDeletePayload{
		Scope:    existing.Scope,
		ScopeID:  existing.ScopeID,
		Key:      existing.Key,
		OldValue: existing.Value,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarDelete,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusOK, pc)
}

// --- Service-scoped handlers ---

func (s *Server) handleCreateServiceEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	if err := s.verifyServiceInProject(r.Context(), projectID, serviceID); err != nil {
		s.errorResponse(w, r, err)
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
	if err := s.checkEnvVarKeyFree(r.Context(), projectID, deploykit.EnvVarScopeService, serviceID, req.Key); err != nil {
		s.errorResponse(w, r, err)
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
		Op:         deploykit.PendingOpEnvVarCreate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &serviceID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusCreated, pc)
}

func (s *Server) handleListServiceEnvVars(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	if err := s.verifyServiceInProject(r.Context(), projectID, serviceID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	envVars, err := s.EnvVarService.ListEnvVars(r.Context(), deploykit.EnvVarScopeService, serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"data": envVars})
}

func (s *Server) handleUpdateServiceEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")
	envVarID := r.PathValue("envVarId")

	if err := s.verifyServiceInProject(r.Context(), projectID, serviceID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	existing, err := s.EnvVarService.GetEnvVar(r.Context(), envVarID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.Scope != deploykit.EnvVarScopeService || existing.ScopeID != serviceID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found."))
		return
	}

	var req deploykit.EnvVarUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if req.Value == nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "value is required."))
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarUpdatePayload{
		Value:    *req.Value,
		OldValue: existing.Value,
		Scope:    existing.Scope,
		ScopeID:  existing.ScopeID,
		Key:      existing.Key,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarUpdate,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusOK, pc)
}

func (s *Server) handleDeleteServiceEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")
	envVarID := r.PathValue("envVarId")

	if err := s.verifyServiceInProject(r.Context(), projectID, serviceID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	existing, err := s.EnvVarService.GetEnvVar(r.Context(), envVarID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.Scope != deploykit.EnvVarScopeService || existing.ScopeID != serviceID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Env var not found."))
		return
	}

	payload, err := json.Marshal(deploykit.EnvVarDeletePayload{
		Scope:    existing.Scope,
		ScopeID:  existing.ScopeID,
		Key:      existing.Key,
		OldValue: existing.Value,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarDelete,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(projectID, pc)
	jsonResponse(w, http.StatusOK, pc)
}

// --- Helpers ---

// checkEnvVarKeyFree returns ECONFLICT if an env var with the given key
// already exists on the target — either as an applied row or as a pending
// env_var.create entry. Staging a collision would pass validation but
// roll back the entire deploy tx when EnvVarService.CreateEnvVar hits the
// unique constraint.
func (s *Server) checkEnvVarKeyFree(
	ctx context.Context,
	projectID string,
	scope deploykit.EnvVarScope,
	scopeID, key string,
) error {
	applied, err := s.EnvVarService.ListEnvVars(ctx, scope, scopeID)
	if err != nil {
		return err
	}
	changes, err := s.PendingChangeService.List(ctx, projectID)
	if err != nil {
		return err
	}
	if envVarKeyTaken(applied, changes, scope, scopeID, key) {
		return deploykit.Errorf(deploykit.ECONFLICT, "Env var %q already exists.", key)
	}
	return nil
}

// envVarKeyTaken is a pure check over applied rows and pending entries.
// Exposed separately so unit tests don't need an HTTP stack.
func envVarKeyTaken(
	applied []*deploykit.EnvVar,
	changes []*deploykit.PendingChange,
	scope deploykit.EnvVarScope,
	scopeID, key string,
) bool {
	for _, ev := range applied {
		if ev.Scope != scope || ev.ScopeID != scopeID || ev.Key != key {
			continue
		}
		// An applied row that's staged for deletion doesn't count — the
		// delete lands first at apply time, so re-adding is valid.
		if pendingDeletesEnvVar(changes, ev.ID) {
			continue
		}
		return true
	}
	for _, c := range changes {
		if c.Op != deploykit.PendingOpEnvVarCreate {
			continue
		}
		// Pending-added services carry their env vars under ParentTempID;
		// applied targets use TargetID. We only match the TargetID case — a
		// parent-temp-id staged env var can't collide with an applied service.
		if c.TargetID == nil || *c.TargetID != scopeID {
			continue
		}
		var p deploykit.EnvVarCreatePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			continue
		}
		if p.Scope == scope && p.Key == key {
			return true
		}
	}
	return false
}

// pendingDeletesEnvVar reports whether a staged env_var.delete targets the
// given applied env var row.
func pendingDeletesEnvVar(changes []*deploykit.PendingChange, envVarID string) bool {
	for _, c := range changes {
		if c.Op == deploykit.PendingOpEnvVarDelete && c.TargetID != nil && *c.TargetID == envVarID {
			return true
		}
	}
	return false
}

// verifyServiceInProject returns ENOTFOUND if the service doesn't exist or
// doesn't belong to the given project.
func (s *Server) verifyServiceInProject(ctx context.Context, projectID, serviceID string) error {
	svc, err := s.ServiceService.GetService(ctx, serviceID)
	if err != nil {
		return err
	}
	if svc.ProjectID != projectID {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Service not found.")
	}
	return nil
}

// currentUserID returns the authenticated user's ID (or nil if unauthenticated).
// Pending change rows record who staged the edit.
func currentUserID(ctx context.Context) *string {
	u := UserFromContext(ctx)
	if u == nil {
		return nil
	}
	return &u.ID
}

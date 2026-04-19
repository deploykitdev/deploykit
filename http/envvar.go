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

	payload, err := json.Marshal(deploykit.EnvVarUpdatePayload{Value: *req.Value})
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

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarDelete,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    json.RawMessage(`{}`),
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

	payload, err := json.Marshal(deploykit.EnvVarUpdatePayload{Value: *req.Value})
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

	pc, err := s.PendingChangeService.Append(r.Context(), projectID, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpEnvVarDelete,
		TargetType: deploykit.PendingTargetEnvVar,
		TargetID:   &envVarID,
		Payload:    json.RawMessage(`{}`),
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

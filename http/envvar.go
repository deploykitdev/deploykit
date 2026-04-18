package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/heyjorgedev/deploykit"
)

// --- Project-scoped handlers ---

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

	ev, err := s.EnvVarService.CreateEnvVar(r.Context(), deploykit.EnvVarScopeProject, projectID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployProjectServices(r.Context(), projectID); err != nil {
		s.logger.Error("redeploying services after project env var create", "err", err, "project_id", projectID)
	}

	jsonResponse(w, http.StatusCreated, ev)
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

	ev, err := s.EnvVarService.UpdateEnvVar(r.Context(), envVarID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployProjectServices(r.Context(), projectID); err != nil {
		s.logger.Error("redeploying services after project env var update", "err", err, "project_id", projectID)
	}

	jsonResponse(w, http.StatusOK, ev)
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

	if err := s.EnvVarService.DeleteEnvVar(r.Context(), envVarID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployProjectServices(r.Context(), projectID); err != nil {
		s.logger.Error("redeploying services after project env var delete", "err", err, "project_id", projectID)
	}

	w.WriteHeader(http.StatusNoContent)
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

	ev, err := s.EnvVarService.CreateEnvVar(r.Context(), deploykit.EnvVarScopeService, serviceID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployService(r.Context(), serviceID); err != nil {
		s.logger.Error("redeploying service after env var create", "err", err, "service_id", serviceID)
	}

	jsonResponse(w, http.StatusCreated, ev)
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

	ev, err := s.EnvVarService.UpdateEnvVar(r.Context(), envVarID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployService(r.Context(), serviceID); err != nil {
		s.logger.Error("redeploying service after env var update", "err", err, "service_id", serviceID)
	}

	jsonResponse(w, http.StatusOK, ev)
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

	if err := s.EnvVarService.DeleteEnvVar(r.Context(), envVarID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := s.redeployService(r.Context(), serviceID); err != nil {
		s.logger.Error("redeploying service after env var delete", "err", err, "service_id", serviceID)
	}

	w.WriteHeader(http.StatusNoContent)
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

// redeployProjectServices creates a new deployment for every service in the
// project that currently has an active deployment, then triggers the
// reconciler once. Services without an active deployment are skipped — they
// have no image to redeploy with and will pick up env vars on their first
// manual deploy.
func (s *Server) redeployProjectServices(ctx context.Context, projectID string) error {
	services, _, err := s.ServiceService.ListServices(ctx, deploykit.ServiceFilter{
		ProjectID: &projectID,
		Limit:     100,
	})
	if err != nil {
		return fmt.Errorf("listing services for project %s: %w", projectID, err)
	}

	anyRedeployed := false
	for _, svc := range services {
		redeployed, err := s.redeployServiceNoTrigger(ctx, svc.ID)
		if err != nil {
			return err
		}
		anyRedeployed = anyRedeployed || redeployed
	}

	if anyRedeployed && s.Reconciler != nil {
		s.Reconciler.Trigger()
	}
	return nil
}

// redeployService creates a new deployment for a single service (if it has an
// active deployment) using the current resolved env var set, and triggers the
// reconciler.
func (s *Server) redeployService(ctx context.Context, serviceID string) error {
	redeployed, err := s.redeployServiceNoTrigger(ctx, serviceID)
	if err != nil {
		return err
	}
	if redeployed && s.Reconciler != nil {
		s.Reconciler.Trigger()
	}
	return nil
}

// redeployServiceNoTrigger creates a new deployment snapshot for the service
// based on its current active deployment, but with freshly resolved env vars.
// Returns (true, nil) if a new deployment was created. Does not trigger the
// reconciler — callers batch that.
func (s *Server) redeployServiceNoTrigger(ctx context.Context, serviceID string) (bool, error) {
	svc, err := s.ServiceService.GetService(ctx, serviceID)
	if err != nil {
		return false, fmt.Errorf("getting service %s: %w", serviceID, err)
	}
	if svc.ActiveDeploymentID == nil {
		return false, nil
	}

	active, err := s.DeploymentService.GetDeployment(ctx, *svc.ActiveDeploymentID)
	if err != nil {
		return false, fmt.Errorf("getting active deployment %s: %w", *svc.ActiveDeploymentID, err)
	}

	resolved, err := s.EnvVarService.ResolveForService(ctx, serviceID)
	if err != nil {
		return false, fmt.Errorf("resolving env vars for service %s: %w", serviceID, err)
	}

	newDep, err := s.DeploymentService.CreateDeployment(ctx, serviceID, deploykit.DeploymentCreate{
		Image:     active.Image,
		EnvVars:   resolved,
		Ports:     active.Ports,
		Resources: active.Resources,
		Replicas:  active.Replicas,
	})
	if err != nil {
		return false, fmt.Errorf("creating redeploy for service %s: %w", serviceID, err)
	}

	if s.EventBus != nil {
		s.EventBus.Publish(ctx, deploykit.Event{
			Type:      deploykit.EventDeploymentCreated,
			ProjectID: svc.ProjectID,
			Payload:   deploykit.DeploymentCreatedPayload{Deployment: newDep},
		})
	}
	return true, nil
}

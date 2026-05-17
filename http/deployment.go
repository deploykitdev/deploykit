package http

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/deploykitdev/deploykit"
)

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceId")

	var req deploykit.DeploymentCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	// Merge project + service env vars under any deploy-time overrides in the
	// request. Request values win so callers can pin canary flags per deploy.
	resolved, err := s.EnvVarService.ResolveForService(r.Context(), serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	maps.Copy(resolved, req.EnvVars)
	req.EnvVars = resolved

	deployment, err := s.DeploymentService.CreateDeployment(r.Context(), serviceID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if s.EventBus != nil {
		// Look up the service to get the ProjectID so subscribers can filter.
		if svc, err := s.ServiceService.GetService(r.Context(), serviceID); err == nil {
			s.EventBus.Publish(r.Context(), deploykit.Event{
				Type:      deploykit.EventDeploymentCreated,
				ProjectID: svc.ProjectID,
				Payload:   deploykit.DeploymentCreatedPayload{Deployment: deployment},
			})
		}
	}

	if s.Reconciler != nil {
		s.Reconciler.TriggerService(serviceID)
	}

	jsonResponse(w, http.StatusCreated, deployment)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := r.PathValue("deploymentId")

	deployment, err := s.DeploymentService.GetDeployment(r.Context(), deploymentID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, deployment)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceId")

	filter := deploykit.DeploymentFilter{
		ServiceID: &serviceID,
	}

	var err error
	if filter.Offset, err = parseQueryInt(r, "offset", 0); err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if filter.Limit, err = parseQueryInt(r, "limit", 0); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	deployments, totalCount, err := s.DeploymentService.ListDeployments(r.Context(), filter)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if deployments == nil {
		deployments = make([]*deploykit.Deployment, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"data":        deployments,
		"total_count": totalCount,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

func (s *Server) handleRollbackService(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceId")

	var req struct {
		DeploymentID string `json:"deployment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if req.DeploymentID == "" {
		ve := deploykit.NewValidationErrors()
		ve.Add("deployment_id", "Deployment ID is required.")
		s.errorResponse(w, r, ve.Err())
		return
	}

	svc, err := s.DeploymentService.RollbackService(r.Context(), serviceID, req.DeploymentID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, svc)
}

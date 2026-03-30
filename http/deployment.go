package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heyjorgedev/deploykit"
)

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceId")

	var req deploykit.DeploymentCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	deployment, err := s.DeploymentService.CreateDeployment(r.Context(), serviceID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
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

	if v := r.URL.Query().Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
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

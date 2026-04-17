package http

import (
	"encoding/json"
	"net/http"

	"github.com/heyjorgedev/deploykit"
)

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	var req deploykit.ServiceCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	svc, err := s.ServiceService.CreateService(r.Context(), projectID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusCreated, svc)
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	svc, err := s.ServiceService.GetService(r.Context(), serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Verify the service belongs to the requested project.
	if svc.ProjectID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found."))
		return
	}

	jsonResponse(w, http.StatusOK, svc)
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	filter := deploykit.ServiceFilter{
		ProjectID: &projectID,
		Name:      parseQueryString(r, "name"),
		Status:    parseQueryString(r, "status"),
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

	services, totalCount, err := s.ServiceService.ListServices(r.Context(), filter)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if services == nil {
		services = make([]*deploykit.Service, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"data":        services,
		"total_count": totalCount,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

func (s *Server) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	// Verify service belongs to project before updating.
	existing, err := s.ServiceService.GetService(r.Context(), serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.ProjectID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found."))
		return
	}

	var req deploykit.ServiceUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	svc, err := s.ServiceService.UpdateService(r.Context(), serviceID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, svc)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	// Verify service belongs to project before deleting.
	existing, err := s.ServiceService.GetService(r.Context(), serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if existing.ProjectID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found."))
		return
	}

	if err := s.ServiceService.DeleteService(r.Context(), serviceID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if s.EventBus != nil {
		s.EventBus.Publish(r.Context(), deploykit.Event{
			Type:      deploykit.EventServiceDeleted,
			ProjectID: projectID,
			Payload:   deploykit.ServiceDeletedPayload{ServiceID: serviceID},
		})
	}

	if s.Reconciler != nil {
		s.Reconciler.Trigger()
	}

	w.WriteHeader(http.StatusNoContent)
}

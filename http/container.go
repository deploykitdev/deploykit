package http

import (
	"net/http"
	"strconv"

	"github.com/heyjorgedev/deploykit"
)

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceId")

	filter := deploykit.ContainerFilter{
		ServiceID: &serviceID,
	}

	if v := r.URL.Query().Get("deployment_id"); v != "" {
		filter.DeploymentID = &v
	}
	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = &v
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}

	containers, totalCount, err := s.ContainerService.ListContainers(r.Context(), filter)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if containers == nil {
		containers = make([]*deploykit.Container, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"data":        containers,
		"total_count": totalCount,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

package http

import (
	"encoding/json"
	"net/http"

	"github.com/deploykitdev/deploykit"
)

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req deploykit.ProjectCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	project, err := s.ProjectService.CreateProject(r.Context(), req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if s.Reconciler != nil {
		s.Reconciler.Trigger()
	}

	jsonResponse(w, http.StatusCreated, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("projectId")

	project, err := s.ProjectService.GetProject(r.Context(), id)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, project)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	filter := deploykit.ProjectFilter{
		Name: parseQueryString(r, "name"),
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

	projects, totalCount, err := s.ProjectService.ListProjects(r.Context(), filter)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if projects == nil {
		projects = make([]*deploykit.Project, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"data":        projects,
		"total_count": totalCount,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("projectId")

	if _, err := s.ProjectService.GetProject(r.Context(), id); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	var req deploykit.ProjectUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if err := req.Validate(); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	payload, err := json.Marshal(deploykit.ProjectUpdatePayload{Name: req.Name})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	pc, err := s.PendingChangeService.Append(r.Context(), id, deploykit.PendingChangeInput{
		Op:         deploykit.PendingOpProjectUpdate,
		TargetType: deploykit.PendingTargetProject,
		TargetID:   &id,
		Payload:    payload,
		UserID:     currentUserID(r.Context()),
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	s.canvasHub.broadcastPendingChangeAdded(id, pc)
	jsonResponse(w, http.StatusOK, pc)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("projectId")

	if err := s.ProjectService.DeleteProject(r.Context(), id); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if s.Reconciler != nil {
		s.Reconciler.Trigger()
	}

	w.WriteHeader(http.StatusNoContent)
}

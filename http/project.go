package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heyjorgedev/deploykit"
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

	jsonResponse(w, http.StatusCreated, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	project, err := s.ProjectService.GetProject(r.Context(), id)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, project)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	var filter deploykit.ProjectFilter

	if v := r.URL.Query().Get("name"); v != "" {
		filter.Name = &v
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
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
	id := r.PathValue("id")

	var req deploykit.ProjectUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	project, err := s.ProjectService.UpdateProject(r.Context(), id, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, project)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.ProjectService.DeleteProject(r.Context(), id); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

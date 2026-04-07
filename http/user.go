package http

import (
	"encoding/json"
	"net/http"

	"github.com/heyjorgedev/deploykit"
)

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req deploykit.UserCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	if req.Role == "" {
		req.Role = deploykit.RoleMember
	}

	user, err := s.UserService.CreateUser(r.Context(), req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusCreated, user)
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	user, err := s.UserService.GetUser(r.Context(), id)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, user)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	filter := deploykit.UserFilter{
		Email: parseQueryString(r, "email"),
	}
	if v := r.URL.Query().Get("role"); v != "" {
		role := deploykit.Role(v)
		filter.Role = &role
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

	users, totalCount, err := s.UserService.ListUsers(r.Context(), filter)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if users == nil {
		users = make([]*deploykit.User, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"data":        users,
		"total_count": totalCount,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req deploykit.UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	user, err := s.UserService.UpdateUser(r.Context(), id, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.UserService.DeleteUser(r.Context(), id); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

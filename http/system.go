package http

import "net/http"

func (s *Server) handleGetAbout(w http.ResponseWriter, r *http.Request) {
	about, err := s.SystemService.About(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, about)
}

func (s *Server) handleGetSystemStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.SystemService.Status(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, status)
}

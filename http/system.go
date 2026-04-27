package http

import (
	"encoding/json"
	"net/http"

	"github.com/heyjorgedev/deploykit"
)

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

func (s *Server) handleGetLatestRelease(w http.ResponseWriter, r *http.Request) {
	// Allow callers to force a refresh with ?refresh=1.
	if r.URL.Query().Get("refresh") == "1" {
		release, err := s.SystemService.RefreshLatestRelease(r.Context())
		if err != nil {
			s.errorResponse(w, r, err)
			return
		}
		jsonResponse(w, http.StatusOK, release)
		return
	}
	release, err := s.SystemService.LatestRelease(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, release)
}

type upgradeRequest struct {
	Version string `json:"version"`
}

func (s *Server) handleRequestUpgrade(w http.ResponseWriter, r *http.Request) {
	var req upgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	if err := s.SystemService.RequestUpgrade(r.Context(), req.Version); err != nil {
		s.errorResponse(w, r, err)
		return
	}
	status, err := s.SystemService.UpgradeStatus(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusAccepted, status)
}

func (s *Server) handleGetUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.SystemService.UpgradeStatus(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, status)
}

func (s *Server) handleGetSystemSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.SystemService.GetSettings(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var update deploykit.SystemSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}
	settings, err := s.SystemService.UpdateSettings(r.Context(), update)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, settings)
}

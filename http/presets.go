package http

import (
	"net/http"
)

// handleListDatabasePresets returns the curated database preset catalog.
// Generators are not run — values are filled in only at Get time so each
// dialog open produces fresh randoms.
func (s *Server) handleListDatabasePresets(w http.ResponseWriter, r *http.Request) {
	presets, err := s.PresetService.List(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, presets)
}

// handleGetDatabasePreset returns one preset with its generators materialized.
// Each call returns freshly generated values for every Generate-typed env var.
func (s *Server) handleGetDatabasePreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("presetId")
	preset, err := s.PresetService.Get(r.Context(), id)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, preset)
}

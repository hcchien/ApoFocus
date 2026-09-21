package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/hcchien/apofocus/internal/deepanalysis"
)

func (s *Server) createDeepAnalysisJob(w http.ResponseWriter, r *http.Request) {
	var input deepanalysis.CreateInput
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, id := range input.PhotoIDs {
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, "photoIds must contain UUIDs")
			return
		}
	}
	job, err := s.deepAnalysis.Create(r.Context(), input)
	if errors.Is(err, deepanalysis.ErrNotFound) {
		writeError(w, http.StatusNotFound, "selected photo not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) getDeepAnalysisJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.deepAnalysis.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, deepanalysis.ErrNotFound) {
		writeError(w, http.StatusNotFound, "deep analysis job not found")
		return
	}
	if err != nil {
		s.internalError(w, "get deep analysis job", err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) getDeepAnalysisItems(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.deepAnalysis.Items(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		s.internalError(w, "get deep analysis items", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getPhotoDeepAnalysis(w http.ResponseWriter, r *http.Request) {
	result, err := s.deepAnalysis.LatestForPhoto(r.Context(), r.PathValue("id"))
	if errors.Is(err, deepanalysis.ErrNotFound) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	if err != nil {
		s.internalError(w, "get photo deep analysis", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) cancelDeepAnalysisJob(w http.ResponseWriter, r *http.Request) {
	err := s.deepAnalysis.Cancel(r.Context(), r.PathValue("id"))
	if errors.Is(err, deepanalysis.ErrNotFound) {
		writeError(w, http.StatusNotFound, "deep analysis job not found")
		return
	}
	if err != nil {
		s.internalError(w, "cancel deep analysis job", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

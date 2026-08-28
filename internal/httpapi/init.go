package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hcchien/apofocus/internal/initjob"
)

func (s *Server) createInitRun(w http.ResponseWriter, r *http.Request) {
	var input initjob.CreateInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid init request")
		return
	}
	run, err := s.initJobs.Create(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}
func (s *Server) getInitRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.initJobs.Get(r.Context(), r.PathValue("id"))
	writeInitResult(w, run, err)
}
func (s *Server) listInitRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.initJobs.List(r.Context(), r.URL.Query().Get("status"), boundedInt(r.URL.Query().Get("limit"), 20, 1, 100))
	if err != nil {
		s.internalError(w, "list init runs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": runs})
}
func (s *Server) getInitItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.initJobs.Items(r.Context(), r.PathValue("id"), boundedInt(r.URL.Query().Get("limit"), 200, 1, 1000))
	if err != nil {
		s.internalError(w, "list init items", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *Server) pauseInitRun(w http.ResponseWriter, r *http.Request) {
	if err := s.initJobs.Pause(r.Context(), r.PathValue("id")); err != nil {
		writeInitError(w, err)
		return
	}
	run, err := s.initJobs.Get(r.Context(), r.PathValue("id"))
	writeInitResult(w, run, err)
}
func (s *Server) resumeInitRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.initJobs.Resume(r.Context(), r.PathValue("id"))
	writeInitResult(w, run, err)
}
func (s *Server) cancelInitRun(w http.ResponseWriter, r *http.Request) {
	if err := s.initJobs.Cancel(r.Context(), r.PathValue("id")); err != nil {
		writeInitError(w, err)
		return
	}
	run, err := s.initJobs.Get(r.Context(), r.PathValue("id"))
	writeInitResult(w, run, err)
}
func writeInitResult(w http.ResponseWriter, run initjob.Run, err error) {
	if err != nil {
		writeInitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
func writeInitError(w http.ResponseWriter, err error) {
	if errors.Is(err, initjob.ErrNotFound) {
		writeError(w, http.StatusNotFound, "init run not found")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

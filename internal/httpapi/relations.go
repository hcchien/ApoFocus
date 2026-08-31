package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/hcchien/apofocus/internal/catalog"
)

type relationEntityInput struct {
	Description string `json:"description"`
}

type projectStoriesInput struct {
	StoryIDs []string `json:"storyIds"`
}

func (s *Server) listRelationCatalog(w http.ResponseWriter, r *http.Request) {
	result, err := s.relations.ListRelationCatalog(r.Context())
	if err != nil {
		s.internalError(w, "list relation catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) bulkUpdateRelations(w http.ResponseWriter, r *http.Request) {
	var input catalog.BulkRelationUpdate
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateBulkRelationUpdate(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.relations.BulkUpdateRelations(r.Context(), input)
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "selected asset or relationship target not found")
		return
	}
	if err != nil {
		s.internalError(w, "bulk update relationships", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	description, ok := relationDescription(w, r)
	if !ok {
		return
	}
	item, err := s.relations.CreateProject(r.Context(), description)
	writeRelationEntity(w, http.StatusCreated, item, err, "project")
}

func (s *Server) updateProjectDescription(w http.ResponseWriter, r *http.Request) {
	description, ok := relationDescription(w, r)
	if !ok {
		return
	}
	item, err := s.relations.UpdateProject(r.Context(), r.PathValue("id"), description)
	writeRelationEntity(w, http.StatusOK, item, err, "project")
}

func (s *Server) replaceProjectStories(w http.ResponseWriter, r *http.Request) {
	var input projectStoriesInput
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRelationIDs("storyIds", &input.StoryIDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.relations.ReplaceProjectStories(r.Context(), r.PathValue("id"), input.StoryIDs)
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		s.internalError(w, "replace project stories", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createStory(w http.ResponseWriter, r *http.Request) {
	description, ok := relationDescription(w, r)
	if !ok {
		return
	}
	item, err := s.relations.CreateStory(r.Context(), description)
	writeRelationEntity(w, http.StatusCreated, item, err, "story")
}

func (s *Server) updateStoryDescription(w http.ResponseWriter, r *http.Request) {
	description, ok := relationDescription(w, r)
	if !ok {
		return
	}
	item, err := s.relations.UpdateStory(r.Context(), r.PathValue("id"), description)
	writeRelationEntity(w, http.StatusOK, item, err, "story")
}

func relationDescription(w http.ResponseWriter, r *http.Request) (string, bool) {
	var input relationEntityInput
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	input.Description = strings.TrimSpace(input.Description)
	if input.Description == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return "", false
	}
	if len([]rune(input.Description)) > 10000 {
		writeError(w, http.StatusBadRequest, "description is too long")
		return "", false
	}
	return input.Description, true
}

func writeRelationEntity[T any](w http.ResponseWriter, status int, item T, err error, kind string) {
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, kind+" not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save "+kind)
		return
	}
	writeJSON(w, status, item)
}

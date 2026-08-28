package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/hcchien/apofocus/internal/catalog"
)

const maxEditBody = 1 << 20

func (s *Server) updatePhoto(w http.ResponseWriter, r *http.Request) {
	var input catalog.PhotoUpdate
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePhotoUpdate(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	photo, err := s.store.Update(r.Context(), r.PathValue("id"), input)
	writeEditResult(w, photo, err, "photo")
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeError(w, http.StatusNotFound, mediaType+" not found")
		return
	}
	var input catalog.MediaUpdate
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMediaUpdate(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	asset, err := s.media.UpdateMedia(r.Context(), mediaType, r.PathValue("id"), input)
	writeEditResult(w, asset, err, mediaType)
}

func decodeEdit(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxEditBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid edit payload: " + err.Error())
	}
	return nil
}

func validatePhotoUpdate(input catalog.PhotoUpdate) error {
	if input.Revision == nil {
		return errors.New("revision is required")
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return errors.New("title cannot be empty")
	}
	if input.Rating != nil && (*input.Rating < 0 || *input.Rating > 5) {
		return errors.New("rating must be between 0 and 5")
	}
	if input.ISO != nil && *input.ISO < 0 {
		return errors.New("iso cannot be negative")
	}
	if input.Location != nil && (input.Location.Latitude < -90 || input.Location.Latitude > 90 || input.Location.Longitude < -180 || input.Location.Longitude > 180) {
		return errors.New("invalid location coordinates")
	}
	if input.Tags != nil && len(*input.Tags) > 100 {
		return errors.New("at most 100 tags are allowed")
	}
	return nil
}

func validateMediaUpdate(input catalog.MediaUpdate) error {
	if input.Revision == nil {
		return errors.New("revision is required")
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return errors.New("title cannot be empty")
	}
	if input.Rating != nil && (*input.Rating < 0 || *input.Rating > 5) {
		return errors.New("rating must be between 0 and 5")
	}
	if input.Tags != nil && len(*input.Tags) > 100 {
		return errors.New("at most 100 tags are allowed")
	}
	return nil
}

func writeEditResult[T any](w http.ResponseWriter, value T, err error, kind string) {
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, kind+" not found")
		return
	}
	if errors.Is(err, catalog.ErrConflict) {
		writeError(w, http.StatusConflict, "record changed; reload before saving")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update "+kind)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
